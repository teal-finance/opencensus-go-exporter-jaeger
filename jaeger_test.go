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

package jaeger

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/uber/jaeger-client-go/thrift-gen/jaeger"
	"go.opencensus.io/trace"
)

// TODO(jbd): Test export.

func Test_bytesToInt64(t *testing.T) {
	type args struct{}
	tests := []struct {
		buf  []byte
		want int64
	}{
		{
			buf:  []byte{255, 0, 0, 0, 0, 0, 0, 0},
			want: -72057594037927936,
		},
		{
			buf:  []byte{0, 0, 0, 0, 0, 0, 0, 1},
			want: 1,
		},
		{
			buf:  []byte{0, 0, 0, 0, 0, 0, 0, 0},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.want), func(t *testing.T) {
			if got := bytesToInt64(tt.buf); got != tt.want {
				t.Errorf("bytesToInt64() = \n%v, \n want \n%v", got, tt.want)
			}
		})
	}
}

func Test_spanDataToThrift(t *testing.T) {
	now := time.Now()

	answerValue := int64(42)
	keyValue := "value"
	resultValue := true
	statusCodeValue := int64(2)
	doubleValue := float64(123.456)
	boolTrue := true
	statusMessage := "error"

	tests := []struct {
		name string
		data *trace.SpanData
		want *jaeger.Span
	}{
		{
			name: "no parent",
			data: &trace.SpanData{
				SpanContext: trace.SpanContext{
					TraceID: trace.TraceID{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					SpanID:  trace.SpanID{1, 2, 3, 4, 5, 6, 7, 8},
				},
				Name:      "/foo",
				StartTime: now,
				EndTime:   now,
				Attributes: map[string]interface{}{
					"double": doubleValue,
					"key":    keyValue,
				},
				Annotations: []trace.Annotation{
					{
						Time:    now,
						Message: statusMessage,
						Attributes: map[string]interface{}{
							"answer": answerValue,
						},
					},
					{
						Time:    now,
						Message: statusMessage,
						Attributes: map[string]interface{}{
							"result": resultValue,
						},
					},
				},
				Status: trace.Status{Code: trace.StatusCodeUnknown, Message: "error"},
			},
			want: &jaeger.Span{
				TraceIdLow:    651345242494996240,
				TraceIdHigh:   72623859790382856,
				SpanId:        72623859790382856,
				OperationName: "/foo",
				StartTime:     now.UnixNano() / 1000,
				Duration:      0,
				Tags: []*jaeger.Tag{
					{Key: "double", VType: jaeger.TagType_DOUBLE, VDouble: &doubleValue},
					{Key: "key", VType: jaeger.TagType_STRING, VStr: &keyValue},
					{Key: "error", VType: jaeger.TagType_BOOL, VBool: &boolTrue},
					{Key: "status.code", VType: jaeger.TagType_LONG, VLong: &statusCodeValue},
					{Key: "status.message", VType: jaeger.TagType_STRING, VStr: &statusMessage},
				},
				Logs: []*jaeger.Log{
					{Timestamp: now.UnixNano() / 1000, Fields: []*jaeger.Tag{
						{Key: "answer", VType: jaeger.TagType_LONG, VLong: &answerValue},
						{Key: "message", VType: jaeger.TagType_STRING, VStr: &statusMessage},
					}},
					{Timestamp: now.UnixNano() / 1000, Fields: []*jaeger.Tag{
						{Key: "result", VType: jaeger.TagType_BOOL, VBool: &resultValue},
						{Key: "message", VType: jaeger.TagType_STRING, VStr: &statusMessage},
					}},
				},
				References: []*jaeger.SpanRef{},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := spanDataToThrift(tt.data)
			sort.Slice(got.Tags, func(i, j int) bool {
				return got.Tags[i].Key < got.Tags[j].Key
			})
			sort.Slice(tt.want.Tags, func(i, j int) bool {
				return tt.want.Tags[i].Key < tt.want.Tags[j].Key
			})
			gotBuf, err := serialize(got)
			if err != nil {
				t.Fatalf("failed to serialize: %v", err)
			}
			wantBuf, err := serialize(tt.want)
			if err != nil {
				t.Fatalf("failed to serialize: %v", err)
			}
			if gotBuf.String() != wantBuf.String() {
				t.Errorf("spanDataToThrift()\nGot:\n%v\nWant;\n%v", got, tt.want)
			}
		})
	}
}
