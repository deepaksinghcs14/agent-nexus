package handler

import "regexp"

var unconfirmedFutureWork = regexp.MustCompile(`(?is)\b(?:i(?:'|’)ll|i will|we(?:'|’)ll|we will)\b[^.]{0,120}\b(?:get back|check|look into|investigate|follow up|share an update|update you|come back)\b|\blet me\s+(?:check|look into|investigate)\b|\b(?:get back|follow up|update you)\b[^.]{0,80}\b(?:later|tomorrow|today|soon|end of day)\b`)

const futureWorkCorrection = "\n\nCritical response policy: your previous draft promised future work without a confirmed tool result. Do not promise follow-up, background work, or a later update. Answer now using available information, or plainly state that you do not have enough information."

func promisesUnconfirmedFutureWork(reply string) bool {
	return unconfirmedFutureWork.MatchString(reply)
}
