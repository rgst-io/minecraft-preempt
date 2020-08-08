package main

import (
	"encoding/json"
	"time"

	"github.com/Tnze/go-mc/bot"
	"github.com/jaredallard/minecraft-preempt/pkg/config"
	"github.com/jaredallard/minecraft-preempt/pkg/instance"

	"github.com/golang/glog"
)

type serverStatus struct {
	Players struct {
		Online int
	}
}

func pingStatus(addr string, port int) (*serverStatus, error) {
	resp, _, err := bot.PingAndListTimeout(addr, port, time.Second*5)
	if err != nil {
		return nil, err
	}

	var s serverStatus
	err = json.Unmarshal(resp, &s)
	if err != nil {
		return nil, err
	}

	return &s, nil
}

func maybeShutdown(conf *config.ProxyConfig, google *instance.Client) error {
	if time.Now().Sub(playersLastSeenAt) < conf.Server.ShutDownAfter {
		return nil
	}

	glog.Info("shutting down instance because there have been no players in a while")
	return google.Stop(conf.Instance.Project, conf.Instance.Zone, conf.Instance.ID)
}
