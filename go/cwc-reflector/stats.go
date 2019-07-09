package main

import (
	"context"
	"time"
)

import "../bitoip"

const ActiveDuration = StationGoneTimeout
const OnlineDuration = StationGoneTimeout

type ChannelActivityInfo struct {
	ChannelId   bitoip.ChannelIdType
	ActiveCalls []string
	OnlineCalls []string
	LastActive  time.Duration
	LastOnline  time.Duration
}

var channelActivity []ChannelActivityInfo

func AssembleChannelActivity() {
	var activities []ChannelActivityInfo
	now := time.Now()

	for cId, c := range channels {

		lastActiveTime := time.Time{}
		lastOnlineTime := time.Time{}
		activeCalls := make([]string, 0)
		onlineCalls := make([]string, 0)

		for _, s := range c.Subscribers {
			if s.LastTx.After(lastActiveTime) {
				lastActiveTime = s.LastTx
			}
			if s.LastListen.After(lastOnlineTime) {
				lastOnlineTime = s.LastListen
			}
			if now.Sub(s.LastTx) < ActiveDuration {
				activeCalls = append(activeCalls, s.Callsign)
			}
			if now.Sub(s.LastListen) < OnlineDuration {
				onlineCalls = append(onlineCalls, s.Callsign)
			}
		}

		act := ChannelActivityInfo{
			cId,
			activeCalls,
			onlineCalls,
			now.Sub(lastActiveTime),
			now.Sub(lastOnlineTime),
		}

		activities = append(activities, act)

	}

	channelActivity = activities
}

func UpdateChannelActivity(ctx context.Context) {
	tick := time.Tick(30 * time.Second)

	for {
		select {
		case <-tick:
			AssembleChannelActivity()
		case <-ctx.Done():
			return
		}
	}
}
