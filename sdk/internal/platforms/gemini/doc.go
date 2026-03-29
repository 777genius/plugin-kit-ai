// Package gemini reserves the Gemini target identity in the internal platform tree.
//
// Gemini is currently scaffold/render/import/validate only. It intentionally has
// no runtime event implementation in the SDK yet, but it still needs a distinct
// internal package so descriptor metadata does not alias Codex internals.
package gemini
