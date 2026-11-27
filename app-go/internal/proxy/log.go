package proxy

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/laposa/tmock/app-go/internal/clientip"
	"github.com/laposa/tmock/app-go/internal/logx"
)

// logRequest logs the tmock.proxy.request event (spec §7.9) and, when the upstream call
// failed, tmock.proxy.upstream_failed first. The message is the line
//
//	<ip> <METHOD> <url> <status>[ [client <c>; scenario <s>; mocked: status, headers, body[; proxy skipped]]] <ms>ms <size>
//
// Header values and bodies are never logged (spec §2.4.3).
func (p *Proxy) logRequest(r *http.Request, status int, size int64, ex *exchange, d time.Duration) {
	ctx := r.Context()
	if ex.failure != nil {
		p.logUpstreamFailed(ctx, ex)
	}
	if !p.logger.Enabled(ctx, slog.LevelInfo) {
		return
	}
	ip := clientip.From(r)
	ms := d.Milliseconds()
	attrs := []slog.Attr{
		slog.String(logx.AttrEventName, logx.EventProxyRequest),
		slog.String(logx.AttrHTTPMethod, r.Method),
		slog.String(logx.AttrURLPath, r.URL.Path),
		slog.Int(logx.AttrHTTPStatusCode, status),
		slog.String(logx.AttrClientAddress, ip),
		slog.Int64(logx.AttrDurationMS, ms),
		slog.Int64(logx.AttrHTTPBodySize, size),
	}
	if r.URL.RawQuery != "" {
		attrs = append(attrs, slog.String(logx.AttrURLQuery, r.URL.RawQuery))
	}
	if ex.service != nil {
		attrs = append(attrs,
			slog.String(logx.AttrService, ex.service.Path),
			slog.String(logx.AttrServerAddress, ex.service.Upstream.Hostname()),
		)
	}
	details, detailAttrs := matchDetails(ex.sel)
	attrs = append(attrs, detailAttrs...)
	msg := fmt.Sprintf("%s %s %s %d%s %dms %s", ip, r.Method, r.URL.RequestURI(), status, details, ms, FormatSize(size))
	p.logger.LogAttrs(ctx, slog.LevelInfo, msg, attrs...)
}

// matchDetails is the bracketed part of the log line and its attributes: the reported
// client and, when a scenario applies, the scenario, what it mocks and whether the
// upstream was skipped. Both are empty when no client matched.
func matchDetails(sel selection) (string, []slog.Attr) {
	if sel.client == nil {
		return "", nil
	}
	parts := []string{"client " + sel.client.Name}
	attrs := []slog.Attr{slog.String(logx.AttrClient, sel.client.Name)}
	if s := sel.scenario; s != nil {
		mocked := mockedParts(s)
		skipped := skipsProxy(s)
		parts = append(parts, "scenario "+s.Name)
		if len(mocked) > 0 {
			parts = append(parts, "mocked: "+strings.Join(mocked, ", "))
		}
		if skipped {
			parts = append(parts, "proxy skipped")
		}
		attrs = append(attrs,
			slog.String(logx.AttrScenario, s.Name),
			slog.Any(logx.AttrMocked, mocked),
			slog.Bool(logx.AttrProxySkipped, skipped),
		)
	}
	return " [" + strings.Join(parts, "; ") + "]", attrs
}

func (p *Proxy) logUpstreamFailed(ctx context.Context, ex *exchange) {
	host := ex.service.Upstream.Hostname()
	p.logger.LogAttrs(ctx, slog.LevelError,
		fmt.Sprintf("upstream of service %q (%s) failed: %v", ex.service.Path, host, ex.failure),
		slog.String(logx.AttrEventName, logx.EventUpstreamFailed),
		slog.String(logx.AttrService, ex.service.Path),
		slog.String(logx.AttrServerAddress, host),
		slog.String(logx.AttrErrorType, errorType(ex.failure)),
		slog.String(logx.AttrExceptionMessage, ex.failure.Error()),
	)
}

// errorType is a short, low-cardinality name for an upstream error (error.type).
func errorType(err error) string {
	var dnsErr *net.DNSError
	var netErr net.Error
	switch {
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, os.ErrDeadlineExceeded):
		return "timeout"
	case errors.As(err, &dnsErr):
		return "dns"
	case errors.Is(err, syscall.ECONNREFUSED):
		return "connection_refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "connection_reset"
	case errors.As(err, &netErr) && netErr.Timeout():
		return "timeout"
	}
	return fmt.Sprintf("%T", err)
}

// FormatSize writes a byte count in a short human-readable form (spec §7.9): 0B, 512B,
// 1.5KB, 2MB, with at most one decimal and powers of 1024.
func FormatSize(n int64) string {
	const unit = 1024
	if n < unit {
		return strconv.FormatInt(n, 10) + "B"
	}
	v := float64(n)
	suffix := ""
	for _, s := range []string{"KB", "MB", "GB", "TB"} {
		v /= unit
		suffix = s
		if v < unit {
			break
		}
	}
	text := strconv.FormatFloat(v, 'f', 1, 64)
	if len(text) > 2 && text[len(text)-2:] == ".0" {
		text = text[:len(text)-2]
	}
	return text + suffix
}
