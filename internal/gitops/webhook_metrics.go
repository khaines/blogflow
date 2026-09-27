package gitops

import "github.com/prometheus/client_golang/prometheus"

// Webhook request outcomes. The set is fixed so the metric has bounded
// cardinality.
const (
	outcomeOK               = "ok"
	outcomeForbiddenIP      = "forbidden_ip"
	outcomeMissingSignature = "missing_signature"
	outcomeInvalidSignature = "invalid_signature"
	outcomeBodyTooLarge     = "body_too_large"
	outcomeBodyReadError    = "body_read_error"
	outcomeFailureBudget    = "failure_budget_exceeded"
	outcomeVerifiedBudget   = "verified_budget_exceeded"
	outcomeMethodNotAllowed = "method_not_allowed"
	outcomeEventRejected    = "event_rejected"
	outcomeInvalidPayload   = "invalid_payload"
	outcomeBranchSkipped    = "branch_skipped"
	outcomeReloadFailed     = "reload_failed"
)

// webhookOutcomes lists every outcome so the series exist from startup.
var webhookOutcomes = []string{
	outcomeOK, outcomeForbiddenIP, outcomeMissingSignature, outcomeInvalidSignature,
	outcomeBodyTooLarge, outcomeBodyReadError, outcomeFailureBudget, outcomeVerifiedBudget,
	outcomeMethodNotAllowed, outcomeEventRejected, outcomeInvalidPayload,
	outcomeBranchSkipped, outcomeReloadFailed,
}

// webhookRequestsTotal counts every webhook request by outcome. Unauthenticated
// rejections beyond the per-IP failures budget are not logged, so this is
// the operator's signal for sustained junk or forgery traffic.
var webhookRequestsTotal = prometheus.NewCounterVec(
	prometheus.CounterOpts{
		Name: "blogflow_webhook_requests_total",
		Help: "Webhook requests by outcome.",
	},
	[]string{"outcome"},
)

func init() {
	prometheus.MustRegister(webhookRequestsTotal)
	for _, o := range webhookOutcomes {
		webhookRequestsTotal.WithLabelValues(o)
	}
}
