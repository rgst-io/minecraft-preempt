package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	nativenet "net"

	"github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/jaredallard/minecraft-preempt/pkg/config"
	"github.com/jaredallard/minecraft-preempt/pkg/instance"

	"github.com/golang/glog"
)

// Target server address
var (
	RemoteServerIP   = ""
	RemoteServerPort = 25565
)

// Cached last status of the server
var (
	cachedStatus = "UNKNOWN"
)

// Client is a minecraft protocol aware connection somewhat
type Client struct {
	net.Conn

	ProtocolVersion int32
}

func (c *Client) handshake() (nextState int32, err error) {
	p, err := c.ReadPacket()
	if err != nil {
		return -1, err
	}
	if p.ID != 0 {
		return -1, fmt.Errorf("packet ID 0x%X is not handshake", p.ID)
	}

	var (
		sid pk.String
		spt pk.Short
	)
	if err := p.Scan(
		(*pk.VarInt)(&c.ProtocolVersion),
		&sid, &spt,
		(*pk.VarInt)(&nextState)); err != nil {
		return -1, err
	}

	return nextState, nil
}

func (c *Client) status(version string, protoVersion int, statusMessage string) {
	for i := 0; i < 2; i++ {
		p, err := c.ReadPacket()
		if err != nil {
			break
		}

		switch p.ID {
		case 0x00:
			respPack := getStatus(version, protoVersion, statusMessage)
			c.WritePacket(respPack)
		case 0x01:
			c.WritePacket(p)
		}
	}
}

// getStatus returns a templated status
func getStatus(version string, protoVersion int, statusMessage string) pk.Packet {
	return pk.Packet{
		ID: 0x00,
		Data: pk.String(fmt.Sprintf(`
		{
			"version": {
				"name": "%s",
				"protocol": %d
			},
			"players": {
				"max": 1,
				"online": 0,
				"sample": []
			},	
			"description": {
				"text": "%s"
			}
		}
		`, version, protoVersion, statusMessage)).Encode(),
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, "usage: example -stderrthreshold=[INFO|WARN|FATAL] -log_dir=[string]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	flag.Usage = usage
	flag.Set("logtostderr", "true")
	flag.Set("v", "2")
	flag.Parse()

	conf, err := config.LoadProxyConfig()
	if err != nil {
		panic(err)
	}

	google := instance.NewClient()

	RemoteServerIP = conf.Server.Hostname
	RemoteServerPort = conf.Server.Port

	// Listen for incoming connections.
	l, err := net.ListenMC("localhost:25565")
	if err != nil {
		fmt.Println("Error listening:", err.Error())
		os.Exit(1)
	}
	defer l.Close()

	for {
		conn, err := l.Accept()
		if err != nil {
			panic(err)
		}
		go Handle(conn, conf, google)
	}
}

// Handle a connection
func Handle(conn net.Conn, conf *config.ProxyConfig, google *instance.Client) {
	defer conn.Close()
	c := Client{Conn: conn}

	nextState, err := c.handshake()
	if err != nil {
		glog.Errorf("handshake failed: %v", err)
		return
	}

	const (
		CheckState  = 1
		PlayerLogin = 2
	)
	switch nextState {
	case CheckState:
		statusMessage := "Server is hibernated. Join to start it."
		if cachedStatus == "RUNNING" { // TODO(jaredallard): get real information here
			statusMessage = "Server is online!"
		}
		c.status(conf.Server.Version, conf.Server.ProtocolVersion, statusMessage)
	case PlayerLogin:
		status, err := google.Status(conf.Instance.Project, conf.Instance.Zone, conf.Instance.ID)
		if err != nil {
			glog.Warningf("failed to get parent instance status: %v", err)
			return
		}

		glog.V(3).Infof("parent instance status is %s", status)

		serverDisconnectPacket := pk.Packet{
			ID: 0x00,
			Data: pk.String(fmt.Sprintf(`
					{
						"translate": "chat.type.text",
						"with": [{
							"text": "Server is currently being launched. (Status: %s)"
						}]
					}
				`, status)).Encode(),
		}

		cachedStatus = status

		// start the instance
		if status == "STOPPED" || status == "TERMINATED" {
			glog.Infof("starting server ...")
			err := c.WritePacket(serverDisconnectPacket)
			if err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}

			if err := google.Start(conf.Instance.Project, conf.Instance.Zone, conf.Instance.ID); err != nil {
				glog.Errorf("failed to start instance: %v", err)
			}
			return
		} else if status != "RUNNING" {
			// tell the client we're waiting for the server to start
			glog.Infof("server is status '%s', waiting ...", status)
			err := c.WritePacket(serverDisconnectPacket)
			if err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}
			return
		}

		glog.Infof("starting proxy to remote '%s' for '%s'", fmt.Sprintf("%s:%d", RemoteServerIP, RemoteServerPort), conn.Socket.RemoteAddr())
		rconn, err := nativenet.Dial("tcp", fmt.Sprintf("%s:%d", RemoteServerIP, RemoteServerPort))
		if err != nil {
			glog.Errorf("failed to open connection to remote: %v", err)
			return
		}

		handshake := pk.Marshal(
			0x00,
			pk.VarInt(c.ProtocolVersion),
			pk.String(RemoteServerIP),
			pk.UnsignedShort(RemoteServerPort),
			pk.Byte(2),
		)
		if _, err = rconn.Write(handshake.Pack(0)); err != nil {
			glog.Errorf("failed to send created handshake: %v", err)
			return
		}

		glog.Infof("sent emulated handshake")

		go func() {
			_, err := io.Copy(conn.Socket, rconn)
			if err != nil {
				glog.Errorf("failed to write to client from remote: %v", err)
				return
			}
		}()

		_, err = io.Copy(rconn, conn.Socket)
		if err != nil {
			glog.Errorf("failed to write to remote from client: %v", err)
			return
		}
	}
}
