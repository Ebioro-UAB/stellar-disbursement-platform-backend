package signing

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/stellar/go-stellar-sdk/keypair"
	"github.com/stellar/go-stellar-sdk/txnbuild"
	"github.com/stellar/go-stellar-sdk/xdr"

	sdpUtils "github.com/stellar/stellar-disbursement-platform-backend/internal/utils"
)

// ExternalTSAOptions holds configuration for connecting to an external TSA service.
type ExternalTSAOptions struct {
	NetworkPassphrase string
	BaseURL           string
	AuthToken         string
	HTTPClient        *http.Client // injectable for tests
}

func (o ExternalTSAOptions) Validate() error {
	if o.NetworkPassphrase == "" {
		return fmt.Errorf("network passphrase cannot be empty")
	}
	if o.BaseURL == "" {
		return fmt.Errorf("TSA base URL cannot be empty")
	}
	return nil
}

// ExternalTSASignatureClient delegates transaction signing to an external TSA HTTP service.
// The SDP builds the transaction XDR, the TSA signs it and returns raw ed25519 signatures,
// and this client attaches those signatures to the transaction envelope.
// Private keys never touch the SDP.
type ExternalTSASignatureClient struct {
	networkPassphrase string
	baseURL           string
	authToken         string
	httpClient        *http.Client
}

var _ SignatureClient = (*ExternalTSASignatureClient)(nil)

// NewExternalTSASignatureClient returns a configured ExternalTSASignatureClient.
func NewExternalTSASignatureClient(opts ExternalTSAOptions) (*ExternalTSASignatureClient, error) {
	if err := opts.Validate(); err != nil {
		return nil, fmt.Errorf("validating ExternalTSA options: %w", err)
	}

	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}

	return &ExternalTSASignatureClient{
		networkPassphrase: opts.NetworkPassphrase,
		baseURL:           opts.BaseURL,
		authToken:         opts.AuthToken,
		httpClient:        httpClient,
	}, nil
}

// tsaSignRequest is the JSON body sent to POST /sign.
type tsaSignRequest struct {
	TransactionXDR    string   `json:"transaction_xdr"`
	NetworkPassphrase string   `json:"network_passphrase"`
	Signers           []string `json:"signers"`
}

// tsaSignResponse is the JSON body received from POST /sign.
type tsaSignResponse struct {
	Signatures []tsaSignature `json:"signatures"`
}

type tsaSignature struct {
	PublicKey string `json:"public_key"`
	Signature string `json:"signature"` // base64-encoded raw ed25519 signature
}

// SignStellarTransaction sends the unsigned XDR to the TSA, receives raw signatures,
// and attaches them to the transaction envelope.
func (c *ExternalTSASignatureClient) SignStellarTransaction(
	ctx context.Context,
	stellarTx *txnbuild.Transaction,
	stellarAccounts ...string,
) (*txnbuild.Transaction, error) {
	if stellarTx == nil {
		return nil, fmt.Errorf("stellarTx cannot be nil in %s", sdpUtils.GetTypeName(c))
	}
	if len(stellarAccounts) == 0 {
		return nil, fmt.Errorf("stellar accounts cannot be empty in %s", sdpUtils.GetTypeName(c))
	}

	txEnvelope := stellarTx.ToXDR()

	sigs, err := c.requestSignatures(ctx, txEnvelope.MarshalBinary, stellarAccounts)
	if err != nil {
		return nil, fmt.Errorf("requesting signatures from TSA: %w", err)
	}

	// Attach each returned signature to the transaction envelope.
	for _, sig := range sigs {
		rawSig, decodeErr := base64.StdEncoding.DecodeString(sig.Signature)
		if decodeErr != nil {
			return nil, fmt.Errorf("decoding base64 signature for %s: %w", sig.PublicKey, decodeErr)
		}

		kp, parseErr := keypair.ParseAddress(sig.PublicKey)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing public key %s: %w", sig.PublicKey, parseErr)
		}

		hint := kp.Hint()
		decoratedSig := xdr.DecoratedSignature{
			Hint:      xdr.SignatureHint(hint),
			Signature: xdr.Signature(rawSig),
		}

		stellarTx, err = stellarTx.AddSignatureDecorated(decoratedSig)
		if err != nil {
			return nil, fmt.Errorf("attaching signature for %s: %w", sig.PublicKey, err)
		}
	}

	return stellarTx, nil
}

// SignFeeBumpStellarTransaction applies the same TSA signing pattern to fee-bump transactions.
func (c *ExternalTSASignatureClient) SignFeeBumpStellarTransaction(
	ctx context.Context,
	feeBumpTx *txnbuild.FeeBumpTransaction,
	stellarAccounts ...string,
) (*txnbuild.FeeBumpTransaction, error) {
	if feeBumpTx == nil {
		return nil, fmt.Errorf("feeBumpTx cannot be nil in %s", sdpUtils.GetTypeName(c))
	}
	if len(stellarAccounts) == 0 {
		return nil, fmt.Errorf("stellar accounts cannot be empty in %s", sdpUtils.GetTypeName(c))
	}

	txEnvelope := feeBumpTx.ToXDR()

	sigs, err := c.requestSignatures(ctx, txEnvelope.MarshalBinary, stellarAccounts)
	if err != nil {
		return nil, fmt.Errorf("requesting signatures from TSA: %w", err)
	}

	for _, sig := range sigs {
		rawSig, decodeErr := base64.StdEncoding.DecodeString(sig.Signature)
		if decodeErr != nil {
			return nil, fmt.Errorf("decoding base64 signature for %s: %w", sig.PublicKey, decodeErr)
		}

		kp, parseErr := keypair.ParseAddress(sig.PublicKey)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing public key %s: %w", sig.PublicKey, parseErr)
		}

		hint := kp.Hint()
		decoratedSig := xdr.DecoratedSignature{
			Hint:      xdr.SignatureHint(hint),
			Signature: xdr.Signature(rawSig),
		}

		feeBumpTx, err = feeBumpTx.AddSignatureDecorated(decoratedSig)
		if err != nil {
			return nil, fmt.Errorf("attaching signature for %s: %w", sig.PublicKey, err)
		}
	}

	return feeBumpTx, nil
}

// BatchInsert is a no-op: the TSA manages its own keys externally.
func (c *ExternalTSASignatureClient) BatchInsert(_ context.Context, number int) ([]string, error) {
	if number <= 0 {
		return nil, fmt.Errorf("number must be greater than 0")
	}
	return nil, fmt.Errorf("BatchInsert called for %s: %w", sdpUtils.GetTypeName(c), ErrUnsupportedCommand)
}

// Delete is a no-op: key lifecycle is managed by the external TSA.
func (c *ExternalTSASignatureClient) Delete(_ context.Context, _ string) error {
	return fmt.Errorf("Delete called for %s: %w", sdpUtils.GetTypeName(c), ErrUnsupportedCommand)
}

func (c *ExternalTSASignatureClient) NetworkPassphrase() string {
	return c.networkPassphrase
}

// requestSignatures sends the XDR and the list of required signers to POST /sign
// and returns the raw signatures from the TSA.
func (c *ExternalTSASignatureClient) requestSignatures(
	ctx context.Context,
	marshalFn func() ([]byte, error),
	signers []string,
) ([]tsaSignature, error) {
	xdrBytes, err := marshalFn()
	if err != nil {
		return nil, fmt.Errorf("marshalling XDR to binary: %w", err)
	}
	xdrBase64 := base64.StdEncoding.EncodeToString(xdrBytes)

	reqBody := tsaSignRequest{
		TransactionXDR:    xdrBase64,
		NetworkPassphrase: c.networkPassphrase,
		Signers:           signers,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshalling TSA request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/sign", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("creating TSA HTTP request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.authToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.authToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Network errors are retryable — the worker will retry on the next cycle.
		return nil, fmt.Errorf("calling TSA /sign: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading TSA response body: %w", err)
	}

	switch {
	case resp.StatusCode >= 500:
		// 5xx — TSA internal error, retryable.
		return nil, fmt.Errorf("TSA returned %d (retryable): %s", resp.StatusCode, string(respBytes))
	case resp.StatusCode >= 400:
		// 4xx — bug in our request construction, terminal error.
		return nil, fmt.Errorf("TSA returned %d (terminal, bad request): %s", resp.StatusCode, string(respBytes))
	case resp.StatusCode != http.StatusOK:
		return nil, fmt.Errorf("TSA returned unexpected status %d: %s", resp.StatusCode, string(respBytes))
	}

	var tsaResp tsaSignResponse
	if err = json.Unmarshal(respBytes, &tsaResp); err != nil {
		return nil, fmt.Errorf("parsing TSA response: %w", err)
	}

	if len(tsaResp.Signatures) == 0 {
		return nil, fmt.Errorf("TSA returned no signatures for signers %v", signers)
	}

	return tsaResp.Signatures, nil
}
