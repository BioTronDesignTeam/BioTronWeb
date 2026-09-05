package model

import "errors"

// ErrRateLimited is the one vendor failure the agent loop answers differently.
// Every other error is "Sprinter could not answer that"; a quota refusal is
// worth telling the operator about, because waiting a minute fixes it.
// A vendor adapter wraps its own error around this, so errors.Is finds it.
var ErrRateLimited = errors.New("model is rate limited")
