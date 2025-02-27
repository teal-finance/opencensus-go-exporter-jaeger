// Copyright 2018, OpenCensus Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package jaeger contains an OpenCensus tracing exporter for Jaeger.
package jaeger // import "github.com/teal-finance/opencensus-go-exporter-jaeger"

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/uber/jaeger-client-go/thrift"
	"github.com/uber/jaeger-client-go/thrift-gen/jaeger"
	"go.opencensus.io/trace"
	"google.golang.org/api/support/bundler"
)

const defaultServiceName = "OpenCensus"

// Options are the options to be used when initializing a Jaeger exporter.
type Options struct {
	// Endpoint is the Jaeger HTTP Thrift endpoint.
	// For example, http://localhost:14268.
	//
	// Deprecated: Use CollectorEndpoint instead.
	Endpoint string

	// CollectorEndpoint is the full url to the Jaeger HTTP Thrift collector.
	// For example, http://localhost:14268/api/traces
	CollectorEndpoint string

	// AgentEndpoint instructs exporter to send spans to jaeger-agent at this address.
	// For example, localhost:6831.
	AgentEndpoint string

	// OnError is the hook to be called when there is
	// an error occurred when uploading the stats data.
	// If no custom hook is set, errors are logged.
	// Optional.
	OnError func(err error)

	// Username to be used if basic auth is required.
	// Optional.
	Username string

	// Password to be used if basic auth is required.
	// Optional.
	Password string

	// ServiceName is the Jaeger service name.
	// Deprecated: Specify Process instead.
	ServiceName string

	// Process contains the information about the exporting process.
	Process Process

	// BufferMaxCount defines the total number of traces that can be buffered in memory
	BufferMaxCount int
}

// NewExporter returns a trace.Exporter implementation that exports
// the collected spans to Jaeger.
func NewExporter(o Options) (*Exporter, error) {
	if o.Endpoint == "" && o.CollectorEndpoint == "" && o.AgentEndpoint == "" {
		return nil, errors.New("missing endpoint for Jaeger exporter")
	}

	var endpoint string
	var client *agentClientUDP
	var err error
	if o.Endpoint != "" {
		endpoint = o.Endpoint + "/api/traces?format=jaeger.thrift"
		log.Printf("Endpoint has been deprecated. Please use CollectorEndpoint instead.")
	} else if o.CollectorEndpoint != "" {
		endpoint = o.CollectorEndpoint
	} else {
		client, err = newAgentClientUDP(o.AgentEndpoint, udpPacketMaxLength)
		if err != nil {
			return nil, err
		}
	}
	onError := func(err error) {
		if o.OnError != nil {
			o.OnError(err)
			return
		}
		log.Printf("Error when uploading spans to Jaeger: %v", err)
	}
	service := o.Process.ServiceName
	if service == "" && o.ServiceName != "" {
		// fallback to old service name if specified
		service = o.ServiceName
	} else if service == "" {
		service = defaultServiceName
	}
	tags := make([]*jaeger.Tag, len(o.Process.Tags))
	for i, tag := range o.Process.Tags {
		tags[i] = attributeToTag(tag.key, tag.value)
	}
	e := &Exporter{
		endpoint:      endpoint,
		agentEndpoint: o.AgentEndpoint,
		client:        client,
		username:      o.Username,
		password:      o.Password,
		process: &jaeger.Process{
			ServiceName: service,
			Tags:        tags,
		},
	}
	bundler := bundler.NewBundler((*jaeger.Span)(nil), func(bundle interface{}) {
		if err := e.upload(bundle.([]*jaeger.Span)); err != nil {
			onError(err)
		}
	})

	// Set BufferedByteLimit with the total number of spans that are permissible to be held in memory.
	// This needs to be done since the size of messages is always set to 1. Failing to set this would allow
	// 1G messages to be held in memory since that is the default value of BufferedByteLimit.
	if o.BufferMaxCount != 0 {
		bundler.BufferedByteLimit = o.BufferMaxCount
	}

	e.bundler = bundler
	return e, nil
}

// Process contains the information exported to jaeger about the source
// of the trace data.
type Process struct {
	// ServiceName is the Jaeger service name.
	ServiceName string

	// Tags are added to Jaeger Process exports
	Tags []Tag
}

// Tag defines a key-value pair
// It is limited to the possible conversions to *jaeger.Tag by attributeToTag.
type Tag struct {
	key   string
	value interface{}
}

// BoolTag creates a new tag of type bool, exported as jaeger.TagType_BOOL.
func BoolTag(key string, value bool) Tag {
	return Tag{key, value}
}

// StringTag creates a new tag of type string, exported as jaeger.TagType_STRING.
func StringTag(key string, value string) Tag {
	return Tag{key, value}
}

// Int64Tag creates a new tag of type int64, exported as jaeger.TagType_LONG.
func Int64Tag(key string, value int64) Tag {
	return Tag{key, value}
}

// Exporter is an implementation of trace.Exporter that uploads spans to Jaeger.
type Exporter struct {
	endpoint      string
	agentEndpoint string
	process       *jaeger.Process
	bundler       *bundler.Bundler
	client        *agentClientUDP

	username, password string
}

var _ trace.Exporter = (*Exporter)(nil)

// ExportSpan exports a SpanData to Jaeger.
func (e *Exporter) ExportSpan(data *trace.SpanData) {
	if data.IsSampled() {
		e.bundler.Add(spanDataToThrift(data), 1)
	}
}

// As per the OpenCensus Status code mapping in
//    https://opencensus.io/tracing/span/status/
// the status is OK if the code is 0.
const opencensusStatusCodeOK = 0

func spanDataToThrift(data *trace.SpanData) *jaeger.Span {
	tags := make([]*jaeger.Tag, 0, len(data.Attributes))
	for k, v := range data.Attributes {
		tag := attributeToTag(k, v)
		if tag != nil {
			tags = append(tags, tag)
		}
	}

	tags = append(tags,
		attributeToTag("status.code", data.Status.Code),
		attributeToTag("status.message", data.Status.Message),
	)

	// Ensure that if Status.Code is not OK, that we set the "error" tag on the Jaeger span.
	// See Issue https://github.com/census-instrumentation/opencensus-go/issues/1041
	if data.Status.Code != opencensusStatusCodeOK {
		tags = append(tags, attributeToTag("error", true))
	}

	logs := make([]*jaeger.Log, 0, len(data.Annotations))
	for _, a := range data.Annotations {
		fields := make([]*jaeger.Tag, 0, len(a.Attributes))
		for k, v := range a.Attributes {
			tag := attributeToTag(k, v)
			if tag != nil {
				fields = append(fields, tag)
			}
		}
		fields = append(fields, attributeToTag("message", a.Message))
		logs = append(logs, &jaeger.Log{
			Timestamp: a.Time.UnixNano() / 1000,
			Fields:    fields,
		})
	}
	refs := make([]*jaeger.SpanRef, 0, len(data.Links))
	for _, link := range data.Links {
		refs = append(refs, &jaeger.SpanRef{
			TraceIdHigh: bytesToInt64(link.TraceID[0:8]),
			TraceIdLow:  bytesToInt64(link.TraceID[8:16]),
			SpanId:      bytesToInt64(link.SpanID[:]),
		})
	}
	return &jaeger.Span{
		TraceIdHigh:   bytesToInt64(data.TraceID[0:8]),
		TraceIdLow:    bytesToInt64(data.TraceID[8:16]),
		SpanId:        bytesToInt64(data.SpanID[:]),
		ParentSpanId:  bytesToInt64(data.ParentSpanID[:]),
		OperationName: name(data),
		Flags:         int32(data.TraceOptions),
		StartTime:     data.StartTime.UnixNano() / 1000,
		Duration:      data.EndTime.Sub(data.StartTime).Nanoseconds() / 1000,
		Tags:          tags,
		Logs:          logs,
		References:    refs,
	}
}

func name(sd *trace.SpanData) string {
	n := sd.Name
	switch sd.SpanKind {
	case trace.SpanKindClient:
		n = "Sent." + n
	case trace.SpanKindServer:
		n = "Recv." + n
	}
	return n
}

func attributeToTag(key string, a interface{}) *jaeger.Tag {
	var tag *jaeger.Tag
	switch value := a.(type) {
	case bool:
		tag = &jaeger.Tag{
			Key:   key,
			VBool: &value,
			VType: jaeger.TagType_BOOL,
		}
	case string:
		tag = &jaeger.Tag{
			Key:   key,
			VStr:  &value,
			VType: jaeger.TagType_STRING,
		}
	case int64:
		tag = &jaeger.Tag{
			Key:   key,
			VLong: &value,
			VType: jaeger.TagType_LONG,
		}
	case int32:
		v := int64(value)
		tag = &jaeger.Tag{
			Key:   key,
			VLong: &v,
			VType: jaeger.TagType_LONG,
		}
	case float64:
		v := float64(value)
		tag = &jaeger.Tag{
			Key:     key,
			VDouble: &v,
			VType:   jaeger.TagType_DOUBLE,
		}
	}
	return tag
}

// Flush waits for exported trace spans to be uploaded.
//
// This is useful if your program is ending and you do not want to lose recent spans.
func (e *Exporter) Flush() {
	e.bundler.Flush()
}

func (e *Exporter) upload(spans []*jaeger.Span) error {
	batch := &jaeger.Batch{
		Spans:   spans,
		Process: e.process,
	}
	if e.endpoint != "" {
		return e.uploadCollector(batch)
	}
	return e.uploadAgent(batch)
}

func (e *Exporter) uploadAgent(batch *jaeger.Batch) error {
	return e.client.EmitBatch(batch)
}

func (e *Exporter) uploadCollector(batch *jaeger.Batch) error {
	body, err := serialize(batch)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", e.endpoint, body)
	if err != nil {
		return err
	}
	if e.username != "" && e.password != "" {
		req.SetBasicAuth(e.username, e.password)
	}
	req.Header.Set("Content-Type", "application/x-thrift")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	io.Copy(ioutil.Discard, resp.Body)
	resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to upload traces; HTTP status code: %d", resp.StatusCode)
	}
	return nil
}

func serialize(obj thrift.TStruct) (*bytes.Buffer, error) {
	buf := thrift.NewTMemoryBuffer()
	if err := obj.Write(context.Background(), thrift.NewTBinaryProtocolTransport(buf)); err != nil {
		return nil, err
	}
	return buf.Buffer, nil
}

func bytesToInt64(buf []byte) int64 {
	u := binary.BigEndian.Uint64(buf)
	return int64(u)
}
