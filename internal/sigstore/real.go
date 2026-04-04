package sigstore

// RealSigner and RealVerifier use sigstore-go against Fulcio and Rekor.
// The real implementation requires network access and is not unit tested.
// It is validated via the manual local demo against Sigstore staging.
//
// This file is a placeholder. The full implementation will be completed
// when the deployment infrastructure (Phase 8) provides the OIDC token
// mechanism needed for server-side keyless signing.
//
// The Signer and Verifier interfaces defined in signer.go are the contract.
// All automated tests use FakeSigner and FakeVerifier from fake.go.
