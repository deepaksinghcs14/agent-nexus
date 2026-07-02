package handler

import (
	"regexp"
	"strings"
)

var unconfirmedFutureWork = regexp.MustCompile(`(?is)\b(?:i(?:'|’)ll|i will|we(?:'|’)ll|we will)\b[^.]{0,120}\b(?:get back|check|look into|investigate|follow up|share an update|update you|come back)\b|\blet me\s+(?:check|look into|investigate)\b|\b(?:get back|follow up|update you)\b[^.]{0,80}\b(?:later|tomorrow|today|soon|end of day)\b`)

const futureWorkCorrection = "\n\nCritical response policy: your previous draft promised future work without a confirmed tool result. Do not promise follow-up, background work, or a later update. Answer now using available information, or plainly state that you do not have enough information."

func promisesUnconfirmedFutureWork(reply string) bool {
	return unconfirmedFutureWork.MatchString(reply)
}

// "[actions taken: …]" is the internal format injectActionLog uses when
// replaying conversation history to the model. A model must never produce it
// itself — when one does (observed with gemini-2.5-pro), it is imitating the
// record of past actions instead of actually calling tools, and everything it
// "reports" (sessions launched, PRs opened) is fabricated.
const fabricatedActionCorrection = "\n\nCritical response policy: your previous draft contained an \"[actions taken: …]\" block. That format is a system-generated record of past activity — you must never write it yourself, and writing it does not execute anything. Perform actions ONLY by emitting real tool calls, then report what the tool results actually say. If you cannot or do not want to call a tool, say so plainly."

// fabricatedActionReply is the honest final answer when the model keeps
// fabricating after a correction retry.
const fabricatedActionReply = "⚠️ The model described tool actions without actually executing them, so the response was discarded — no sessions were launched and no pull requests were created. Please retry the request."

func fabricatesActionLog(reply string) bool {
	return strings.Contains(reply, "[actions taken:")
}
