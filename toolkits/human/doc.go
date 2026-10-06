// Package human provides conversational human review and clarification intents.
// A valid execution delivers a bounded typed PauseSignal and returns toolsy.ErrPause.
// Host owns continuation and authenticated grant issuance for actual operations.
// Neither a free-text intent nor a human/model response grants execution authority.
package human
