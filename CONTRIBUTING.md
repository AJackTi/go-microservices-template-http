# Contributing

This repo is organized as a Go workspace with one service per module.

Before opening a PR:

1. Copy `.env.example` to `.env`.
2. Run `make doctor`.
3. Run `make verify-local`.
4. Run the relevant phase gate:
   - `make verify-phase-1`
   - `make verify-phase-4`
   - `make verify-phase-5`
   - `make verify-phase-6`

Keep changes small and phase-scoped. This project prefers vertical slices over
bulk refactors.
