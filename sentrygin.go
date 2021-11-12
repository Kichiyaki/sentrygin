package sentrygin

import (
	"context"
	"github.com/getsentry/sentry-go"
	"github.com/gin-gonic/gin"
	"net/http"
	"time"
)

// Options configure a Handler.
type Options struct {
	// Repanic configures whether Sentry should repanic after recovery, in most cases it should be set to true,
	// as gin.Default includes its own Recovery middleware what handles http responses.
	Repanic bool
	// WaitForDelivery indicates, in case of a panic, whether to block the
	// current goroutine and wait until the panic event has been reported to
	// Sentry before repanicking or resuming normal execution.
	//
	// This option is normally not needed. Unless you need different behaviors
	// for different HTTP handlers, configure the SDK to use the
	// HTTPSyncTransport instead.
	//
	// Waiting (or using HTTPSyncTransport) is useful when the web server runs
	// in an environment that interrupts execution at the end of a request flow,
	// like modern serverless platforms.
	WaitForDelivery bool
	// Timeout for the delivery of panic events. Defaults to 2s. Only relevant
	// when WaitForDelivery is true.
	//
	// If the timeout is reached, the current goroutine is no longer blocked
	// waiting, but the delivery is not canceled.
	Timeout time.Duration
}

type handler struct {
	repanic         bool
	waitForDelivery bool
	timeout         time.Duration
}

// New returns a function that satisfies gin.HandlerFunc interface
// It can be used with Use() methods.
func New(opts Options) gin.HandlerFunc {
	if opts.Timeout == 0 {
		opts.Timeout = 2 * time.Second
	}

	return (&handler{
		repanic:         opts.Repanic,
		timeout:         opts.Timeout,
		waitForDelivery: opts.WaitForDelivery,
	}).handle
}

func (h *handler) handle(c *gin.Context) {
	ctx := c.Request.Context()

	hub := sentry.GetHubFromContext(ctx)
	if hub == nil {
		hub = sentry.CurrentHub().Clone()
		ctx = sentry.SetHubOnContext(ctx, hub)
	}

	span := sentry.StartSpan(
		ctx,
		"http.server",
		sentry.TransactionName(c.Request.Method+" "+c.Request.URL.Path),
		sentry.ContinueFromRequest(c.Request),
	)
	defer span.Finish()

	c.Request = c.Request.WithContext(span.Context())
	hub.Scope().SetRequest(c.Request)

	defer h.recoverWithSentry(hub, c.Request)

	c.Next()
}

func (h *handler) recoverWithSentry(hub *sentry.Hub, r *http.Request) {
	if err := recover(); err != nil {
		eventID := hub.RecoverWithContext(
			context.WithValue(r.Context(), sentry.RequestContextKey, r),
			err,
		)
		if eventID != nil && h.waitForDelivery {
			hub.Flush(h.timeout)
		}
		if h.repanic {
			panic(err)
		}
	}
}
