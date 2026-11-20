package httpapi

import (
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"proxima.la/sello/sign/internal/auth"
	"proxima.la/sello/sign/internal/model"
	"proxima.la/sello/sign/internal/ratelimit"
	"proxima.la/sello/sign/internal/store"
)

// maxRequestsPerMinute bounds the rate a space or a key sets for itself.
const maxRequestsPerMinute = 1_000_000

// requestsPerMinuteOf is the signing rate of the space: its plan's
// ceiling (in a space without a plan, the install's or the console's),
// lowered by what the space set for itself; nil is none.
func requestsPerMinuteOf(ws store.Workspace) *int {
	ceiling, own := ws.Limits.RequestsPerMinute, ws.OwnRequestsPerMinute
	switch {
	case ceiling == nil:
		return own
	case own == nil || *own > *ceiling:
		return ceiling
	}
	return own
}

// monthlyCap is how many signatures the space may make in env in a
// month: its hard cap (for a paid plan, past its quota: the overage that
// billing charges) or, without one, its quota; nil is none.
func monthlyCap(ws store.Workspace, env model.Environment) *int {
	l := ws.Limits
	if env == model.Prod {
		if l.ProdHardCap != nil {
			return l.ProdHardCap
		}
		return l.ProdSignatures
	}
	if l.TestHardCap != nil {
		return l.TestHardCap
	}
	return l.TestSignatures
}

// rateBuckets are the rate limits a signature counts against: its key's,
// then its space's.
func rateBuckets(ws store.Workspace, k *auth.Key) []ratelimit.Bucket {
	var out []ratelimit.Bucket
	if k != nil && k.RequestsPerMinute != nil {
		out = append(out, ratelimit.Bucket{Name: "key:" + k.ID, Limit: *k.RequestsPerMinute})
	}
	if l := requestsPerMinuteOf(ws); l != nil {
		out = append(out, ratelimit.Bucket{Name: "workspace:" + ws.ID, Limit: *l})
	}
	return out
}

func seconds(d time.Duration) string {
	return strconv.Itoa(max(1, int(math.Ceil(d.Seconds()))))
}

// rateHeaders tell the caller where it stands in the tightest limit.
func rateHeaders(w http.ResponseWriter, res ratelimit.Result) {
	h := w.Header()
	h.Set("X-RateLimit-Limit", strconv.Itoa(res.Bucket.Limit))
	h.Set("X-RateLimit-Remaining", strconv.Itoa(res.Remaining))
	h.Set("X-RateLimit-Reset", seconds(res.Reset))
}

// nextMonth is the first instant of the month after now's, in UTC: when
// the monthly counts start again.
func nextMonth(now time.Time) time.Time {
	u := now.UTC()
	return time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
}

func environmentName(env model.Environment) string {
	if env == model.Prod {
		return "producción"
	}
	return "prueba"
}

// admit is what a signature must pass, in the space that signs, before
// it's made: the space is open, the rate limits of the key (k, nil from
// the panel) and of the space allow it, and the space hasn't reached what
// its plan allows this month in env. What's signed through a delegation
// counts in the space that signs: signer is that space.
func (s *Server) admit(w http.ResponseWriter, r *http.Request, signer string, env model.Environment, k *auth.Key) bool {
	ctx := r.Context()
	ws, err := s.Store.Workspace(ctx, signer)
	if err != nil {
		internalError(w, r, err)
		return false
	}
	if ws.Closed() {
		writeProblem(w, problem(http.StatusForbidden, "workspace-closed", "La organización de este espacio se cerró en Sello: ya no firma."))
		return false
	}
	now := s.Now()
	if buckets := rateBuckets(ws, k); len(buckets) > 0 {
		res, err := s.Limiter.Take(ctx, now, buckets...)
		switch {
		case err != nil:
			// The limiter is a protection, not a dependency: without it,
			// signing goes on.
			slog.WarnContext(ctx, "rate limiter unavailable; signing without it", "workspace_id", ws.ID, "error", err)
		case !res.Allowed:
			rateHeaders(w, res)
			w.Header().Set("Retry-After", seconds(res.Reset))
			writeProblem(w, rateLimited(res))
			return false
		default:
			rateHeaders(w, res)
		}
	}
	if limit := monthlyCap(ws, env); limit != nil {
		used, err := s.Store.MonthUsage(ctx, ws.ID, env, model.Month(now))
		if err != nil {
			internalError(w, r, err)
			return false
		}
		if used >= int64(*limit) {
			resets := nextMonth(now)
			w.Header().Set("Retry-After", seconds(resets.Sub(now)))
			writeProblem(w, monthlyLimitReached(*limit, env, resets))
			return false
		}
	}
	return true
}

func rateLimited(res ratelimit.Result) *Problem {
	whose, appliesTo := "del espacio", "workspace"
	if strings.HasPrefix(res.Bucket.Name, "key:") {
		whose, appliesTo = "de la clave", "api_key"
	}
	p := problem(http.StatusTooManyRequests, "rate-limited",
		fmt.Sprintf("Alcanzó el límite de %d firmas por minuto %s. Vuelva a intentarlo en %s segundos.", res.Bucket.Limit, whose, seconds(res.Reset)))
	limit := res.Bucket.Limit
	p.Limit, p.AppliesTo = &limit, appliesTo
	return p
}

func monthlyLimitReached(limit int, env model.Environment, resets time.Time) *Problem {
	p := problem(http.StatusTooManyRequests, "monthly-limit-reached",
		fmt.Sprintf("Alcanzó el límite de %d firmas de %s de este mes del plan del espacio. La cuenta empieza de nuevo el %s.",
			limit, environmentName(env), resets.Format("2006-01-02")))
	p.Limit, p.Environment, p.ResetsAt = &limit, env, &resets
	return p
}

func organizationLimitReached(ws store.Workspace) *Problem {
	p := problem(http.StatusConflict, "organization-limit-reached", "Alcanzó el límite de organizaciones del plan del espacio.")
	if l := ws.Limits.Organizations; l != nil {
		p.Detail = fmt.Sprintf("Alcanzó el límite de %d organizaciones del plan del espacio.", *l)
		p.Limit = l
	}
	return p
}
