package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

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
	glog.Info("Handling status request")
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

// sendDisconnect sends a disconnect packet to the client
func sendDisconnect(mc *minecraft.Client, reason string) error {
	disconnect := map[string]interface{}{
		"translate": "chat.type.text",
		"with": []interface{}{
			map[string]interface{}{
				"text": reason,
			},
		},
	}

	b, err := json.Marshal(disconnect)
	if err != nil {
		return err
	}

	return mc.WritePacket(pk.Marshal(0x00, pk.String(string(b))))
}

// handle handles minecraft connections
func handle(ctx context.Context, conn mcnet.Conn, s *config.ServerConfig, instanceID string, cld cloud.Provider) {
	c := &minecraft.Client{Conn: &conn}
	defer c.Close()

	nextState, originalHandshake, err := c.Handshake()
	if err != nil {
		// don't log EOF
		if !errors.Is(err, io.EOF) {
			glog.Errorf("handshake failed: %v", err)
		}
		return
	}

	switch nextState {
	default:
		glog.Errorf("unknown next state: %d", nextState)
		return
	case CheckState:
		if err := sendStatus(s, c); err != nil {
			glog.Errorf("failed to send status: %v", err)
		}
		return
	case PlayerLogin:
		glog.Infof("Starting proxy session with %q", conn.Socket.RemoteAddr())

		// start the instance, if needed
		switch cachedStatus {
		case cloud.StatusRunning:
			// do nothing, we'll just proxy the connection
		case cloud.StatusStopped:
			glog.Infof("starting server ...")
			if err := sendDisconnect(c, "Server is starting"); err != nil {
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
			if err := sendDisconnect(c, fmt.Sprintf("Waiting for server to start (Status: %q)", cachedStatus)); err != nil {
				glog.Warningf("failed to send starting packet: %v", err)
			}
			return
		}

		glog.Infof("Creating connection to '%s:%d'", s.Hostname, s.Port)
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

		glog.Info("Piping client <-> remote")
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
			glog.Info("Polling server status ...")
			status, err := cloudProvider.Status(ctx, instanceID)
			if err != nil {
				glog.Warningf("failed to get parent instance status: %v", err)
				return
			}
			glog.Info("Server status: ", status)

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
