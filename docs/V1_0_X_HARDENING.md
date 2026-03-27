# `v1.0.x` Hardening Loop

This document defines the allowed scope immediately after `v1.0`.

## Goal

Stabilize the newly promoted `public-stable` contract in production reality before opening `v1.1` scope.

## Allowed Scope

- bug fixes inside approved stable surfaces
- docs clarifications
- diagnostics tightening that does not break stable failure families
- release automation hardening
- Codex smoke reliability improvements that do not widen the contract promise
- install path and checksum hardening inside the existing contract
- migration and deprecation follow-through for beta leftovers

## Disallowed Scope

- new stable APIs
- new hooks or new platforms
- unified API experiments in stable surface
- breaking rename or removal in approved stable set
- widening the stable promise without a new planning cycle

## First Post-`v1` Backlog

1. collect first-wave user feedback on approved stable surfaces
2. carry `plugin-kit-ai install` from local compatibility matrix coverage to refreshed live tarball/unsupported evidence
3. improve Codex runner story only as operational reliability work
4. keep beta leftovers intentional and documented
5. begin `v1.1` planning only after at least one quiet `v1.0.x` cycle
