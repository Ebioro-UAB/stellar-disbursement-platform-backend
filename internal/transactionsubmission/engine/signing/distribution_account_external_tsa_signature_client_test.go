package signing

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/network"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestTransaction creates a minimal valid Stellar transaction for use in tests.
func buildTestTransaction(t *testing.T, sourceKP *keypair.Full) *txnbuild.Transaction {
	t.Helper()
	tx, err := txnbuild.NewTransaction(txnbuild.TransactionParams{
		SourceAccount: &txnbuild.SimpleAccount{
			AccountID: sourceKP.Address(),
			Sequence:  1,
		},
		IncrementSequenceNum: true,
		Operations: []txnbuild.Operation{
			&txnbuild.Payment{
				Destination: keypair.MustRandom().Address(),
				Amount:      "1",
				Asset:       txnbuild.NativeAsset{},
			},
		},
		BaseFee: txnbuild.MinBaseFee,
		Preconditions: txnbuild.Preconditions{
			TimeBounds: txnbuild.NewInfiniteTimeout(),
		},
	})
	require.NoError(t, err)
	return tx
}

func TestExternalTSASignatureClient_Validate(t *testing.T) {
	t.Run("fails with empty network passphrase", func(t *testing.T) {
		_, err := NewExternalTSASignatureClient(ExternalTSAOptions{
			BaseURL: "http://localhost:9000",
		})
		require.ErrorContains(t, err, "network passphrase cannot be empty")
	})

	t.Run("fails with empty base URL", func(t *testing.T) {
		_, err := NewExternalTSASignatureClient(ExternalTSAOptions{
			NetworkPassphrase: network.TestNetworkPassphrase,
		})
		require.ErrorContains(t, err, "TSA base URL cannot be empty")
	})

	t.Run("succeeds with valid options", func(t *testing.T) {
		client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
			NetworkPassphrase: network.TestNetworkPassphrase,
			BaseURL:           "http://localhost:9000",
		})
		require.NoError(t, err)
		assert.Equal(t, network.TestNetworkPassphrase, client.NetworkPassphrase())
	})
}

func TestExternalTSASignatureClient_SignStellarTransaction_HappyPath(t *testing.T) {
	sourceKP := keypair.MustRandom()
	signerKP := keypair.MustRandom()

	// The TSA mock: signs the tx locally and returns the raw signature in base64.
	tsaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/sign", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		var req tsaSignRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		assert.Equal(t, network.TestNetworkPassphrase, req.NetworkPassphrase)
		require.Contains(t, req.Signers, signerKP.Address())

		// Decode the XDR and sign it with the signer KP to produce a real signature.
		xdrBytes, err := base64.StdEncoding.DecodeString(req.TransactionXDR)
		require.NoError(t, err)
		xdrB64 := base64.StdEncoding.EncodeToString(xdrBytes)

		txEnv, err := txnbuild.TransactionFromXDR(xdrB64)
		require.NoError(t, err)
		innerTx, ok := txEnv.Transaction()
		require.True(t, ok)

		signed, err := innerTx.Sign(network.TestNetworkPassphrase, signerKP)
		require.NoError(t, err)

		sigs := signed.ToXDR().Signatures()
		require.NotEmpty(t, sigs)
		rawSig := []byte(sigs[0].Signature)

		resp := tsaSignResponse{
			Signatures: []tsaSignature{
				{
					PublicKey: signerKP.Address(),
					Signature: base64.StdEncoding.EncodeToString(rawSig),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer tsaServer.Close()

	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           tsaServer.URL,
		AuthToken:         "test-token",
	})
	require.NoError(t, err)

	tx := buildTestTransaction(t, sourceKP)
	signedTx, err := client.SignStellarTransaction(context.Background(), tx, signerKP.Address())
	require.NoError(t, err)
	require.NotNil(t, signedTx)
	assert.NotEmpty(t, signedTx.ToXDR().Signatures())
}

func TestExternalTSASignatureClient_SignStellarTransaction_TSA5xxIsRetryable(t *testing.T) {
	tsaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"internal server error"}`))
	}))
	defer tsaServer.Close()

	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           tsaServer.URL,
	})
	require.NoError(t, err)

	kp := keypair.MustRandom()
	tx := buildTestTransaction(t, kp)
	_, err = client.SignStellarTransaction(context.Background(), tx, kp.Address())
	require.ErrorContains(t, err, "TSA returned 500 (retryable)")
}

func TestExternalTSASignatureClient_SignStellarTransaction_TSA4xxIsTerminal(t *testing.T) {
	tsaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad request"}`))
	}))
	defer tsaServer.Close()

	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           tsaServer.URL,
	})
	require.NoError(t, err)

	kp := keypair.MustRandom()
	tx := buildTestTransaction(t, kp)
	_, err = client.SignStellarTransaction(context.Background(), tx, kp.Address())
	require.ErrorContains(t, err, "TSA returned 400 (terminal, bad request)")
}

func TestExternalTSASignatureClient_SignStellarTransaction_TimeoutIsRetryable(t *testing.T) {
	tsaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer tsaServer.Close()

	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           tsaServer.URL,
		HTTPClient:        &http.Client{Timeout: 10 * time.Millisecond},
	})
	require.NoError(t, err)

	kp := keypair.MustRandom()
	tx := buildTestTransaction(t, kp)
	_, err = client.SignStellarTransaction(context.Background(), tx, kp.Address())
	require.ErrorContains(t, err, "calling TSA /sign")
}

func TestExternalTSASignatureClient_SignStellarTransaction_InvalidBase64SignatureIsTerminal(t *testing.T) {
	tsaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := tsaSignResponse{
			Signatures: []tsaSignature{
				{PublicKey: keypair.MustRandom().Address(), Signature: "!!!not-valid-base64!!!"},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer tsaServer.Close()

	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           tsaServer.URL,
	})
	require.NoError(t, err)

	kp := keypair.MustRandom()
	tx := buildTestTransaction(t, kp)
	_, err = client.SignStellarTransaction(context.Background(), tx, kp.Address())
	require.ErrorContains(t, err, "decoding base64 signature")
}

func TestExternalTSASignatureClient_SignFeeBumpStellarTransaction_HappyPath(t *testing.T) {
	sourceKP := keypair.MustRandom()
	feeAccountKP := keypair.MustRandom()

	// Build inner transaction (must be signed before wrapping in fee-bump).
	innerTx := buildTestTransaction(t, sourceKP)
	signedInnerTx, err := innerTx.Sign(network.TestNetworkPassphrase, sourceKP)
	require.NoError(t, err)

	feeBumpTx, err := txnbuild.NewFeeBumpTransaction(txnbuild.FeeBumpTransactionParams{
		Inner:      signedInnerTx,
		FeeAccount: feeAccountKP.Address(),
		BaseFee:    txnbuild.MinBaseFee * 10,
	})
	require.NoError(t, err)

	tsaServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req tsaSignRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&req))
		require.Contains(t, req.Signers, feeAccountKP.Address())

		// Sign locally to produce a real ed25519 signature for the fee-bump envelope.
		xdrBytes, decErr := base64.StdEncoding.DecodeString(req.TransactionXDR)
		require.NoError(t, decErr)
		xdrB64 := base64.StdEncoding.EncodeToString(xdrBytes)

		parsed, parseErr := txnbuild.TransactionFromXDR(xdrB64)
		require.NoError(t, parseErr)
		fb, ok := parsed.FeeBump()
		require.True(t, ok)

		signed, signErr := fb.Sign(network.TestNetworkPassphrase, feeAccountKP)
		require.NoError(t, signErr)

		sigs := signed.ToXDR().Signatures()
		require.NotEmpty(t, sigs)
		rawSig := []byte(sigs[0].Signature)

		resp := tsaSignResponse{
			Signatures: []tsaSignature{
				{
					PublicKey: feeAccountKP.Address(),
					Signature: base64.StdEncoding.EncodeToString(rawSig),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
	defer tsaServer.Close()

	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           tsaServer.URL,
	})
	require.NoError(t, err)

	signedFeeBump, err := client.SignFeeBumpStellarTransaction(context.Background(), feeBumpTx, feeAccountKP.Address())
	require.NoError(t, err)
	require.NotNil(t, signedFeeBump)
	assert.NotEmpty(t, signedFeeBump.ToXDR().Signatures())
}

func TestExternalTSASignatureClient_SignFeeBumpStellarTransaction_NilTxReturnsError(t *testing.T) {
	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           "http://localhost:9000",
	})
	require.NoError(t, err)

	_, err = client.SignFeeBumpStellarTransaction(context.Background(), nil, keypair.MustRandom().Address())
	require.ErrorContains(t, err, "feeBumpTx cannot be nil")
}

func TestExternalTSASignatureClient_BatchInsert_IsUnsupported(t *testing.T) {
	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           "http://localhost:9000",
	})
	require.NoError(t, err)

	_, err = client.BatchInsert(context.Background(), 1)
	require.ErrorIs(t, err, ErrUnsupportedCommand)
}

func TestExternalTSASignatureClient_Delete_IsUnsupported(t *testing.T) {
	client, err := NewExternalTSASignatureClient(ExternalTSAOptions{
		NetworkPassphrase: network.TestNetworkPassphrase,
		BaseURL:           "http://localhost:9000",
	})
	require.NoError(t, err)

	err = client.Delete(context.Background(), keypair.MustRandom().Address())
	require.ErrorIs(t, err, ErrUnsupportedCommand)
}
