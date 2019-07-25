/*
Copyright 2018 Alauda Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package sinks

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/golang/glog"
	"gopkg.in/olivere/elastic.v6"
	api_v1 "k8s.io/api/core/v1"
)

type ElasticSearchConf struct {
	SinkCommonConf
	Endpoint string
	User     string
	Password string
}

type ElasticSearchSink struct {
	config          *ElasticSearchConf
	esClient        *elastic.Client
	beforeFirstList bool
	currentBuffer   []*api_v1.Event
	logEntryChannel chan *api_v1.Event
	// Channel for controlling how many requests are being sent at the same
	// time. It's empty initially, each request adds an object at the start
	// and takes it out upon completion. Channel's capacity is set to the
	// maximum level of parallelism, so any extra request will lock on addition.
	concurrencyChannel chan struct{}
	timer              *time.Timer
	fakeTimeChannel    chan time.Time
}

func DefaultElasticSearchConf() *ElasticSearchConf {
	return &ElasticSearchConf{
		SinkCommonConf: SinkCommonConf{
			FlushDelay:     defaultFlushDelay,
			MaxBufferSize:  defaultMaxBufferSize,
			MaxConcurrency: defaultMaxConcurrency,
		},
	}
}

func NewElasticSearchSink(config *ElasticSearchConf) (*ElasticSearchSink, error) {
	esClient, err := elastic.NewClient(elastic.SetSniff(false),
		elastic.SetHealthcheckTimeoutStartup(10*time.Second), elastic.SetURL(config.Endpoint))
	if err != nil {
		glog.Errorf("Error create elasticsearch(%s) output %v", config.Endpoint, err)
		return nil, err
	}

	glog.Infof("NewElasticSearchOut inited.")

	return &ElasticSearchSink{
		esClient:           esClient,
		beforeFirstList:    true,
		logEntryChannel:    make(chan *api_v1.Event, config.MaxBufferSize),
		config:             config,
		currentBuffer:      []*api_v1.Event{},
		timer:              nil,
		fakeTimeChannel:    make(chan time.Time),
		concurrencyChannel: make(chan struct{}, config.MaxConcurrency),
	}, nil
}

func (es *ElasticSearchSink) OnAdd(event *api_v1.Event) {
	ReceivedEntryCount.WithLabelValues(event.Source.Component).Inc()
	glog.Infof("OnAdd %v", event)
	es.logEntryChannel <- event
}

func (es *ElasticSearchSink) OnUpdate(oldEvent *api_v1.Event, newEvent *api_v1.Event) {
	var oldCount int32
	if oldEvent != nil {
		oldCount = oldEvent.Count
	}

	if newEvent.Count != oldCount+1 {
		// Sink doesn't send a LogEntry to Stackdriver, b/c event compression might
		// indicate that part of the watch history was lost, which may result in
		// multiple events being compressed. This may create an unecessary
		// flood in Stackdriver. Also this is a perfectly valid behavior for the
		// configuration with empty backing storage.
		glog.V(2).Infof("Event count has increased by %d != 1.\n"+
			"\tOld event: %+v\n\tNew event: %+v", newEvent.Count-oldCount, oldEvent, newEvent)
	}
	glog.Infof("OnUpdate %v", newEvent)

	ReceivedEntryCount.WithLabelValues(newEvent.Source.Component).Inc()

	es.logEntryChannel <- newEvent
}

func (es *ElasticSearchSink) OnDelete(*api_v1.Event) {
	// Nothing to do here
}

func (es *ElasticSearchSink) OnList(list *api_v1.EventList) {
	// Nothing to do else
	glog.Infof("OnList %v", list)
	if es.beforeFirstList {
		es.beforeFirstList = false
	}
}

func (es *ElasticSearchSink) Run(stopCh <-chan struct{}) {
	glog.Info("Starting Elasticsearch sink")
	for {
		select {
		case entry := <-es.logEntryChannel:
			es.currentBuffer = append(es.currentBuffer, entry)
			if len(es.currentBuffer) >= es.config.MaxBufferSize {
				es.flushBuffer()
			} else if len(es.currentBuffer) == 1 {
				es.setTimer()
			}
			break
		case <-es.getTimerChannel():
			es.flushBuffer()
			break
		case <-stopCh:
			glog.Info("Elasticsearch sink recieved stop signal, waiting for all requests to finish")
			glog.Info("All requests to Elasticsearch finished, exiting Elasticsearch sink")
			return
		}
	}
}

func (es *ElasticSearchSink) flushBuffer() {
	entries := es.currentBuffer
	es.currentBuffer = nil
	es.concurrencyChannel <- struct{}{}
	go es.sendEntries(entries)
}
func (es *ElasticSearchSink) sendEntries(entries []*api_v1.Event) {
	glog.V(4).Infof("Sending %d entries to Elasticsearch", len(entries))

	currentTime := time.Now()
	currentDate := fmt.Sprintf("%v", currentTime.Format("2006-01-02"))
	bulkRequest := es.esClient.Bulk().Index(eventsLogName + "-" + currentDate)

	for _, event := range entries {
		glog.Infof("Orig obj: %v", event.InvolvedObject)
		newIndex := elastic.NewBulkIndexRequest().Type(eventsLogName).Id(string(event.ObjectMeta.UID)).Doc(event)
		glog.V(4).Infof("Index request on wire: %v", newIndex.String())
		bulkRequest = bulkRequest.Add(newIndex)
	}

	bulkResponse, err := bulkRequest.Do(context.TODO()) //TODO: investigate use of context here
	b, _ := json.Marshal(bulkResponse)
	if err != nil {
		glog.Errorf("save events error: %v", err)
		glog.Errorf("Response object: %v", string(b))
	} else {
		SuccessfullySentEntryCount.Add(float64(len(entries)))
		glog.V(4).Infof("Successfully sent %d entries to Elasticsearch", len(entries))
		glog.V(4).Infof("Response object: %v", string(b))
	}

	failed := bulkResponse.Failed()
	if len(failed) != 0 {
		for _, item := range failed {
			glog.Errorf(string(item.Error.Reason))
		}
	}
	<-es.concurrencyChannel
}

func (es *ElasticSearchSink) setTimer() {
	if es.timer == nil {
		es.timer = time.NewTimer(es.config.FlushDelay)
	} else {
		es.timer.Reset(es.config.FlushDelay)
	}
}

func (es *ElasticSearchSink) getTimerChannel() <-chan time.Time {
	if es.timer == nil {
		return es.fakeTimeChannel
	}
	return es.timer.C
}
