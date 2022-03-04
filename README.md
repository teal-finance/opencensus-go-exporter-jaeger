

# OpenCensus Go Jaeger Exporter

[![Build Status](https://travis-ci.org/census-ecosystem/opencensus-go-exporter-jaeger.svg?branch=master)](https://travis-ci.org/census-ecosystem/opencensus-go-exporter-jaeger) [![GoDoc][godoc-image]][godoc-url]

Provides OpenCensus exporter support for Jaeger.

## 🛑 This library is not maintained

You should migrate to [OpenTelemetry Jaeger Exporter](https://go.opentelemetry.io/otel/exporters/jaeger).

This library depends on [github.com/uber/jaeger-client-go](https://github.com/jaegertracing/jaeger-client-go) which is DEPRECATED and [its documentation](https://www.jaegertracing.io/docs/1.31/client-libraries/#deprecating-jaeger-clients) says:

> Jaeger clients are being retired.
>
> ## Deprecating Jaeger clients
>
> The Jaeger clients have faithfully served our community for several years. We pioneered many new features, such as remotely controlled samplers and per-operation / adaptive sampling, which were critical to the success of distributed tracing deployments at large organizations. However, now that the larger community in OpenTelemetry has caught up with the Jaeger clients in terms of feature parity and there is full support for exporting data to Jaeger, we believe it is time to decommission Jaeger’s native clients and refocus the efforts on the OpenTelemetry SDKs.
>
> For new applications, we recommend using the OpenTelemetry APIs and SDKs. For existing applications that are already instrumented with the OpenTracing API, we recommend replacing the Jaeger clients with the corresponding OpenTelemetry SDKs and the OpenTracing shim/bridge available in most languages supported by Jaeger.
>
> ## Timeline
> 
> We plan to continue accepting pull requests and making new releases of Jaeger clients through the end of 2021. In January 2022 we will enter a code freeze period for 6 months, during which time we will no longer accept pull requests with new features, with the exception of security-related fixes. After that we will archive the client library repositories and will no longer accept new changes.

## Installation

```
$ go get -u github.com/teal-finance/opencensus-go-exporter-jaeger
```

[godoc-image]: https://godoc.org/github.com/teal-finance/opencensus-go-exporter-jaeger?status.svg
[godoc-url]: https://godoc.org/github.com/teal-finance/opencensus-go-exporter-jaeger
