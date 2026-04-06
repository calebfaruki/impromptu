package sigstore

// RekorVerifier queries the Rekor transparency log for verification.
// The real implementation requires network access and is not unit tested.
// It is validated via the manual local demo against Sigstore staging.
//
// Implementation will use sigstore-go's Rekor client to:
// 1. Query entry by log index
// 2. Verify inclusion proof
// 3. Extract signer identity from the certificate SAN
// 4. Extract signed digest from the entry body
// 5. Compare against expected digest
