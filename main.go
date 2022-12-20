package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/function61/gokit/io/bidipipe"
	"github.com/golang/glog"

	mcnet "github.com/Tnze/go-mc/net"
	pk "github.com/Tnze/go-mc/net/packet"

	"github.com/jaredallard/minecraft-preempt/pkg/cloud"
	"github.com/jaredallard/minecraft-preempt/pkg/cloud/docker"
	"github.com/jaredallard/minecraft-preempt/pkg/cloud/gcp"

	"github.com/jaredallard/minecraft-preempt/pkg/config"
	"github.com/jaredallard/minecraft-preempt/pkg/minecraft"
)

var (
	configPath = flag.String("configPath", "config/config.yaml", "Configuration File")
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

func sendStatus(sconf *config.ServerConfig, mc *minecraft.Client) error {
	status := &minecraft.Status{
		Version: &minecraft.StatusVersion{
			Name:     sconf.Version,
			Protocol: int(sconf.ProtocolVersion),
		},
		Players: &minecraft.StatusPlayers{
			Max:    0,
			Online: 0,
		},
		Description: &minecraft.StatusDescription{
			Text: "",
		},
	}

	switch cachedStatus {
	case cloud.StatusRunning:
		newStatus, err := minecraft.GetServerStatus(sconf.Hostname, sconf.Port)
		if err != nil {
			status.Description.Text = "Server is online, but failed to proxy status"
		} else {
			status = newStatus
			glog.Info("Returning status from server: ", spew.Sdump(status))
		}
	case cloud.StatusStarting:
		status.Description.Text = "Server is starting, please wait!"
	case cloud.StatusStopping:
		status.Description.Text = "Server is stopping, please wait to start it!"
	default:
		status.Description.Text = "Server is hibernated. Join to start it."
	}

	return mc.SendStatus(status)
}

// handle handles minecraft connections
func handle(ctx context.Context, conn mcnet.Conn, s *config.ServerConfig, instanceID string, cld cloud.Provider) {
	c := minecraft.Client{Conn: &conn}
	defer c.Close()

	nextState, originalHandshake, err := c.Handshake()
	if err != nil {
		glog.Errorf("handshake failed: %v", err)
		return
	}
	glog.Infof("Read handshake, next state: %d", nextState)

	switch nextState {
	default:
		glog.Errorf("unknown next state: %d", nextState)
		return
	case CheckState:
		if err := sendStatus(s, &c); err != nil {
			glog.Errorf("failed to send status: %v", err)
		}
		return
	case PlayerLogin:
		glog.Infof("starting proxy: '%s' <-> '%s'", fmt.Sprintf("%s:%d", s.Hostname, s.Port), conn.Socket.RemoteAddr())

		serverDisconnectPacket := pk.Marshal(0x00, pk.String(fmt.Sprintf(`
			{
				"translate": "chat.type.text",
				"with": [{
					"text": "Server is currently being launched. (Status: %s)"
				}]
			}
		`, cachedStatus)))

		// start the instance
		switch cachedStatus {
		case cloud.StatusRunning:
			// do nothing
		case cloud.StatusStopped:
			glog.Infof("starting server ...")
			if err := c.WritePacket(serverDisconnectPacket); err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}

			if err := cld.Start(ctx, instanceID); err != nil {
				glog.Errorf("failed to start instance: %v", err)
				return
			}

			// update the status
			cachedStatus = cloud.StatusStarting
			return
		default: // not running or stopped, so we're starting or stopping
			glog.Infof("server is status '%s', waiting ...", cachedStatus)
			if err := c.WritePacket(serverDisconnectPacket); err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}
			return
		}

		glog.Infof("opening connection to '%s:%d'", s.Hostname, s.Port)
		rconn, err := mcnet.DialMC(s.Hostname + ":" + strconv.Itoa(int(s.Port)))
		if err != nil {
			glog.Errorf("failed to open connection to remote: %v", err)
			return
		}
		defer rconn.Close()

		// send the original handshake packet then pipe the rest
		glog.Info("Replaying original handshake packet")
		if err := rconn.WritePacket(*originalHandshake); err != nil {
			glog.Errorf("failed to write handshake to remote: %v", err)
		}

		if err := bidipipe.Pipe(
			bidipipe.WithName("client", conn.Socket),
			bidipipe.WithName("remote", rconn),
		); err != nil {
			glog.Errorf("failed to write to remote from client: %v", err)
			return
		}
	}
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	flag.Usage = usage
	if err := flag.Set("logtostderr", "true"); err != nil {
		glog.Fatalf("failed to set logtostderr: %v", err)
	}
	if err := flag.Set("v", "2"); err != nil {
		glog.Fatalf("failed to set v: %v", err)
	}
	flag.Parse()

	conf, err := config.LoadProxyConfig(*configPath)
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
	glog.Infof("Creating proxy on '%s'", conf.ListenAddress)
	l, err := minecraft.ListenMC(conf.ListenAddress)
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
