package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	pk "github.com/Tnze/go-mc/net/packet"
	"github.com/jaredallard/minecraft-preempt/pkg/cloud"
	"github.com/jaredallard/minecraft-preempt/pkg/cloud/docker"
	gcp "github.com/jaredallard/minecraft-preempt/pkg/cloud/gcp"
	"github.com/jaredallard/minecraft-preempt/pkg/config"
	"github.com/jaredallard/minecraft-preempt/pkg/minecraft"

	"github.com/golang/glog"
)

// Cached last status of the server
var (
	cachedStatus = cloud.StatusUnknown
)

const (
	CheckState = iota + 1
	PlayerLogin
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: minecraft-preempt -stderrthreshold=[INFO|WARN|FATAL] -log_dir=[string]\n")
	flag.PrintDefaults()
	os.Exit(2)
}

// handle handles minecraft connections
func handle(ctx context.Context, conn minecraft.Conn, s *config.ServerConfig, instanceID string, cld cloud.Provider) {
	defer conn.Close()
	c := minecraft.Client{Conn: conn}

	nextState, err := c.Handshake()
	if err != nil {
		glog.Errorf("handshake failed: %v", err)
		return
	}

	status := &minecraft.Status{
		Version: &minecraft.StatusVersion{
			Name:     s.Version,
			Protocol: int(s.ProtocolVersion),
		},
		Players: &minecraft.StatusPlayers{
			Max:    0,
			Online: 0,
		},
		Description: &minecraft.StatusDescription{
			Text: "",
		},
	}

	switch nextState {
	case CheckState:
		switch cachedStatus {
		case cloud.StatusRunning:
			newStatus, err := minecraft.GetServerStatus(s.Hostname, s.Port)
			if err != nil {
				status.Description.Text = "Server is online, but failed to proxy status"
			} else {
				status = newStatus
			}
		case cloud.StatusStarting:
			status.Description.Text = "Server is starting, please wait!"
		case cloud.StatusStopping:
			status.Description.Text = "Server is stopping, please wait to start it!"
		default:
			status.Description.Text = "Server is hibernated. Join to start it."
		}

		c.SendStatus(status)
	case PlayerLogin:
		glog.Infof("starting proxy: '%s' <-> '%s'", fmt.Sprintf("%s:%d", s.Hostname, s.Port), conn.Socket.RemoteAddr())

		serverDisconnectPacket := pk.Packet{
			ID: 0x00,
			Data: pk.String(fmt.Sprintf(`
					{
						"translate": "chat.type.text",
						"with": [{
							"text": "Server is currently being launched. (Status: %s)"
						}]
					}
				`, cachedStatus)).Encode(),
		}

		// start the instance
		if cachedStatus == cloud.StatusStopped {
			glog.Infof("starting server ...")
			err := c.WritePacket(serverDisconnectPacket)
			if err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}

			if err := cld.Start(ctx, instanceID); err != nil {
				glog.Errorf("failed to start instance: %v", err)
			} else {
				cachedStatus = cloud.StatusStarting
			}
			return
		}

		if cachedStatus != cloud.StatusRunning {
			// tell the client we're waiting for the server to start
			glog.Infof("server is status '%s', waiting ...", cachedStatus)
			err := c.WritePacket(serverDisconnectPacket)
			if err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}
			return
		}

		glog.V(3).Infof("opening connection to '%s:%d'", s.Hostname, s.Port)
		rconn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", s.Hostname, s.Port))
		if err != nil {
			glog.Errorf("failed to open connection to remote: %v", err)
			return
		}
		defer rconn.Close()

		handshake := pk.Marshal(
			0x00,
			pk.VarInt(c.ProtocolVersion),
			pk.String(s.Hostname),
			pk.UnsignedShort(s.Port),
			pk.Byte(2),
		)

		// we need to send a handshake packet to the remote server
		// since we consumed the clients. We could potentially just
		// "replay" the one sent by the client connecting to us
		// instead.
		if _, err = rconn.Write(handshake.Pack(0)); err != nil {
			glog.Errorf("failed to send created handshake: %v", err)
			return
		}

		go func() {
			_, err := io.Copy(conn.Socket, rconn)
			if err != nil {
				glog.Errorf("failed to write to client from remote: %v", err)
				return
			}
		}()

		_, err = io.Copy(rconn, conn.ByteReader)
		if err != nil {
			glog.Errorf("failed to write to remote from client: %v", err)
			return
		}
	}
}

func main() {
	// TODO(jaredallard): implement context cancellation
	ctx, cancel := context.WithCancel(context.Background())
	_ = cancel

	flag.Usage = usage
	flag.Set("logtostderr", "true")
	flag.Set("v", "2")
	flag.Parse()

	conf, err := config.LoadProxyConfig()
	if err != nil {
		glog.Fatalf("failed to load config: %v", err)
	}

	var cloudProvider cloud.Provider
	var instanceID string

	switch conf.Cloud {
	case config.CloudGCP:
		instanceID = conf.CloudConfig.GCP.InstanceID
		cloudProvider, err = gcp.NewClient(context.Background(), conf.CloudConfig.GCP.Project, conf.CloudConfig.GCP.Zone)
	case config.CloudDocker:
		instanceID = conf.CloudConfig.Docker.ContainerID
		cloudProvider, err = docker.NewClient()
	default:
		err = fmt.Errorf("unknown cloud provider")
	}
	if err != nil {
		glog.Fatalf("failed to create cloud provider '%s': %v", conf.Cloud, err)
	}

	if instanceID == "" {
		glog.Fatalf("missing instance id")
	}

	// Listen for incoming connections.
	l, err := minecraft.ListenMC("0.0.0.0:25565")
	if err != nil {
		glog.Fatalf("failed to start proxy: %v\n", err)
	}
	defer l.Close()

	// update the cached status every 5 minutes
	go func() {
		for {
			status, err := cloudProvider.Status(ctx, instanceID)
			if err != nil {
				glog.Warningf("failed to get parent instance status: %v", err)
				return
			}

			if cachedStatus != status {
				glog.Infof("server transitioned from '%s' -> '%s'", cachedStatus, status)
				cachedStatus = status
			}

			time.Sleep(5 * time.Minute)
		}
	}()

	for {
		conn, err := l.Accept()
		if err != nil {
			glog.Errorf("failed to accept connection: %v", err)
			continue
		}
		go handle(ctx, conn, conf.Server, instanceID, cloudProvider)
	}
}
