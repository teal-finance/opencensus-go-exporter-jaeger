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
	"context"
	"fmt"
	"io"
	"net"

	"github.com/uber/jaeger-client-go/thrift"
	"github.com/uber/jaeger-client-go/thrift-gen/agent"
	"github.com/uber/jaeger-client-go/thrift-gen/jaeger"
	"github.com/uber/jaeger-client-go/thrift-gen/zipkincore"
)

// udpPacketMaxLength is the max size of UDP packet we want to send, synced with jaeger-agent.
const udpPacketMaxLength = 65000

// agentClientUDP is a UDP client to Jaeger agent that implements gen.Agent interface.
type agentClientUDP struct {
	agent.Agent
	io.Closer

	connUDP       *net.UDPConn
	client        *agent.AgentClient
	maxPacketSize int                   // max size of datagram in bytes
	thriftBuffer  *thrift.TMemoryBuffer // buffer used to calculate byte size of a span
}

// newAgentClientUDP creates a client that sends spans to Jaeger Agent over UDP.
func newAgentClientUDP(hostPort string, maxPacketSize int) (*agentClientUDP, error) {
	if maxPacketSize == 0 {
		maxPacketSize = udpPacketMaxLength
	}

	thriftBuffer := thrift.NewTMemoryBufferLen(maxPacketSize)
	protocolFactory := thrift.NewTCompactProtocolFactory()
	client := agent.NewAgentClientFactory(thriftBuffer, protocolFactory)

	destAddr, err := net.ResolveUDPAddr("udp", hostPort)
	if err != nil {
		return nil, err
	}

	connUDP, err := net.DialUDP(destAddr.Network(), nil, destAddr)
	if err != nil {
		return nil, err
	}
	if err := connUDP.SetWriteBuffer(maxPacketSize); err != nil {
		return nil, err
	}

	clientUDP := &agentClientUDP{
		connUDP:       connUDP,
		client:        client,
		maxPacketSize: maxPacketSize,
		thriftBuffer:  thriftBuffer,
	}
	return clientUDP, nil
}

// EmitBatch implements EmitBatch() of Agent interface.
func (a *agentClientUDP) EmitBatch(batch *jaeger.Batch) error {
	a.thriftBuffer.Reset()
	// 	a.client.SeqId = 0 // we have no need for distinct SeqIds for our one-way UDP messages
	if err := a.client.EmitBatch(context.Background(), batch); err != nil {
		return err
	}
	if a.thriftBuffer.Len() > a.maxPacketSize {
		return fmt.Errorf("Data does not fit within one UDP packet; size %d, max %d, spans %d",
			a.thriftBuffer.Len(), a.maxPacketSize, len(batch.Spans))
	}
	_, err := a.connUDP.Write(a.thriftBuffer.Bytes())
	return err
}

// EmitZipkinBatch implements EmitZipkinBatch() of Agent interface.
func EmitZipkinBatch(spans []*zipkincore.Span) (err error) {
	return fmt.Errorf("not implemented")
}

// Close implements Close() of io.Closer and closes the underlying UDP connection.
func (a *agentClientUDP) Close() error {
	return a.connUDP.Close()
}
