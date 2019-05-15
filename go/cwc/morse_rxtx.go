/*
Copyright (C) 2019 Graeme Sutherland, Nodestone Limited


This program is free software: you can redistribute it and/or modify
it under the terms of the GNU General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU General Public License for more details.

You should have received a copy of the GNU General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/
package cwc

import (
	"github.com/golang/glog"
	"context"
	"sort"
	"sync"
	"time"
)
import "../bitoip"

/**
 * Morse hardware receiver and sender
 *
 * Takes incoming morse (a bit going high and low) and turns it into
 * CarrierBitEvents to send.
 *
 * Based on a regular tick that samples the input and builds a buffer
 */

const Ms = int64(1e6)
const Us = int64(1000)
const DefaultTickTime = time.Duration(5 * Ms)
const MaxSendTimespan = time.Duration(1000 * Ms)
const BreakinTime = time.Duration(100 * Ms)
const MaxEvents = 100

var TickTime = DefaultTickTime
var SendWait = MaxSendTimespan

var LastBit bool = false

type Event struct {
	startTime time.Time
	bitEvent bitoip.BitEvent
}

var events = make([]Event, 0, MaxEvents)
var RxMutex = sync.Mutex{}

var ticker *time.Ticker
var done = make(chan bool)


func SetTickTime(tt time.Duration) {
	TickTime = tt
}

func SetSendWait(sw time.Duration) {
	SendWait = sw
}

var localEcho bool

var channelId bitoip.ChannelIdType

func SetChannelId(cId bitoip.ChannelIdType) {
	channelId = cId
}

func ChannelId () bitoip.ChannelIdType {
	return channelId
}

var carrierKey bitoip.CarrierKeyType

func SetCarrierKey(ck bitoip.CarrierKeyType) {
	carrierKey = ck
}

func CarrierKey() bitoip.CarrierKeyType {
	return carrierKey
}

var timeOffset = int64(0)

func SetTimeOffset(t int64) {
	timeOffset = t
}

var roundTrip = int64(0)

func SetRoundTrip(t int64) {
	roundTrip = t
}


func RunMorseRx(ctx context.Context, morseIO IO, toSend chan bitoip.CarrierEventPayload, echo bool,
	channel bitoip.ChannelIdType) {
	localEcho = echo
	channelId = channel
	LastBit = false // make sure turned off to begin -- the default state
	ticker = time.NewTicker(TickTime)

	Startup(morseIO)

	for {
		select {
		case <- done:
			ticker.Stop()
			return

		case t := <-ticker.C:
			Sample(t, toSend, morseIO)
		}
	}
}

func Stop(morseIO IO) {
	done <- true
	LastBit = false
	morseIO.Close()
}

func Startup(morseIO IO) {
	err := morseIO.Open()
	if err != nil {
		glog.Fatalf("Can't access Morse hardware: %s", err)
	}
}

// Sample input
// TODO should have some sort of back-off if not used recently for power saving
func Sample(t time.Time, toSend chan bitoip.CarrierEventPayload, morseIO IO) {

	TransmitToHardware(t, morseIO)

	rxBit := morseIO.Bit()
	if rxBit != LastBit {
		// change so record it
		LastBit = rxBit
		morseIO.SetToneOut(rxBit)

		var bit uint8 = 0

		if rxBit {
			bit = 1
		}
		RxMutex.Lock()
		events = append(events, Event{t, bitoip.BitEvent(bit) })
		RxMutex.Unlock()
		if  (len(events) >= MaxEvents - 1) && (events[len(events)-1].bitEvent & bitoip.BitOn == 0) {
			events = Flush(events, toSend)
			return
		}
	}
	if len(events)> 0 &&
		(events[len(events)-1].bitEvent & bitoip.BitOn == 0) &&
		((t.Sub(events[0].startTime) >= MaxSendTimespan) ||
		(t.Sub(events[len(events) - 1].startTime) >= BreakinTime)) {
		events = Flush(events, toSend)
	}
}


// Flush events into an output stream
func Flush(events []Event, toSend chan bitoip.CarrierEventPayload) []Event {
	glog.V(2).Infof("Flushing events %v", events)
	RxMutex.Lock()
	if len(events) > 0 {
		toSend <- BuildPayload(events)
		events = events[:0]
	}
	RxMutex.Unlock()
	return events
}

func BuildPayload(events []Event) bitoip.CarrierEventPayload {
	baseTime := events[0].startTime.UnixNano()
	cep := bitoip.CarrierEventPayload{
		channelId,
		carrierKey,
		baseTime + timeOffset,
		[bitoip.MaxBitEvents]bitoip.CarrierBitEvent{},
		time.Now().UnixNano(),
	}
	for i, event := range events {
		bit := event.bitEvent

		// mark last event this message
		if i == (len(events) - 1) {
			bit = bit | bitoip.LastEvent
		}

		cep.BitEvents[i] = bitoip.CarrierBitEvent{
			uint32(event.startTime.UnixNano() - baseTime),
			bit,
		}
	}
	return cep
}

/**
 * Transmitting morse out a gpio pin
 */


var TxMutex = sync.Mutex{}
var TxQueue = make([]Event, 100)

// Queue this stuff for sending... Basically add to queue
// that will be sent out based on the tick timing
func QueueForTransmit(carrierEvents *bitoip.CarrierEventPayload) {
	if (localEcho || (carrierEvents.CarrierKey != carrierKey)) &&
		carrierEvents.Channel == channelId {
		// compose into events
		newEvents := make([]Event, 0)

		// remove the calculated server time offset
		start := time.Unix(0, carrierEvents.StartTimeStamp - timeOffset + roundTrip + int64(MaxSendTimespan))

		for _, ce := range carrierEvents.BitEvents {
			newEvents = append(newEvents, Event{
				start.Add(time.Duration(ce.TimeOffset)),
				ce.BitEvent,
			})
			if (ce.BitEvent & bitoip.LastEvent) > 0 {
				break
			}
		}
		TxMutex.Lock()

		TxQueue = append(TxQueue, newEvents...)

		sort.Slice(TxQueue, func(i, j int) bool { return TxQueue[i].startTime.Before(TxQueue[j].startTime) })

		TxMutex.Unlock()
	} else {
		glog.V(2).Infof("ignoring own carrier")
	}
	glog.V(2).Infof("TXQueue is now: %v", TxQueue)
}


func TransmitToHardware(t time.Time, morseIO IO) {
	now := time.Now()

	TxMutex.Lock()

	if len(TxQueue) > 0 && TxQueue[0].startTime.Before(now) {
		be := TxQueue[0].bitEvent
		morseIO.SetBit(!((be & bitoip.BitOn) == 0))
		TxQueue = TxQueue[1:]
	}

	TxMutex.Unlock()
}