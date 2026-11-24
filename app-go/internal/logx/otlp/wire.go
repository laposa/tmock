package otlp

// The OTLP/HTTP JSON encoding of ExportLogsServiceRequest. Only stable fields are used.
// Per the OTLP JSON mapping, 64-bit integers are decimal strings and trace/span ids are
// lower-case hex (not base64).

type exportRequest struct {
	ResourceLogs []resourceLogs `json:"resourceLogs"`
}

type resourceLogs struct {
	Resource  resource    `json:"resource"`
	ScopeLogs []scopeLogs `json:"scopeLogs"`
}

type resource struct {
	Attributes []keyValue `json:"attributes"`
}

type scopeLogs struct {
	Scope      scope       `json:"scope"`
	LogRecords []logRecord `json:"logRecords"`
}

type scope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type logRecord struct {
	TimeUnixNano         string     `json:"timeUnixNano,omitempty"`
	ObservedTimeUnixNano string     `json:"observedTimeUnixNano"`
	SeverityNumber       int        `json:"severityNumber"`
	SeverityText         string     `json:"severityText"`
	Body                 anyValue   `json:"body"`
	Attributes           []keyValue `json:"attributes,omitempty"`
	EventName            string     `json:"eventName,omitempty"`
	TraceID              string     `json:"traceId,omitempty"`
	SpanID               string     `json:"spanId,omitempty"`
	Flags                uint32     `json:"flags,omitempty"`
}

type keyValue struct {
	Key   string   `json:"key"`
	Value anyValue `json:"value"`
}

// anyValue has exactly one field set.
type anyValue struct {
	StringValue *string     `json:"stringValue,omitempty"`
	BoolValue   *bool       `json:"boolValue,omitempty"`
	IntValue    *string     `json:"intValue,omitempty"`
	DoubleValue *float64    `json:"doubleValue,omitempty"`
	ArrayValue  *arrayValue `json:"arrayValue,omitempty"`
}

type arrayValue struct {
	Values []anyValue `json:"values"`
}

// exportResponse is the part of ExportLogsServiceResponse that matters.
type exportResponse struct {
	PartialSuccess *struct {
		// int64 in protobuf, so a JSON string; some servers send a number.
		RejectedLogRecords jsonInt `json:"rejectedLogRecords"`
		ErrorMessage       string  `json:"errorMessage"`
	} `json:"partialSuccess"`
}
