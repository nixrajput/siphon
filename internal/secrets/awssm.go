package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/nixrajput/siphon/internal/errs"
)

// smGetter is the slice of the Secrets Manager API this backend needs, so tests
// can supply a fake without a real AWS account or network.
type smGetter interface {
	GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput, optFns ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error)
}

// AWSSM resolves "awssm://..." refs from AWS Secrets Manager, reusing the
// standard AWS credential chain (env / shared config / instance role) — the
// same chain the S3 storage backend uses, so no separate credential wiring.
//
// Ref shapes:
//   - awssm://<secret-id>           → the secret's whole SecretString
//   - awssm://<secret-id>#<key>     → field <key> of the secret's JSON object
//
// The #<key> selector matters because Secrets Manager secrets are commonly JSON
// (e.g. {"username":...,"password":...}); without it a ref would resolve to the
// entire JSON blob rather than the one field a DSN needs.
type AWSSM struct {
	client smGetter
}

// NewAWSSM builds an AWS Secrets Manager backend. region may be empty to defer
// to the credential chain's region. Construction is lazy-free (the SDK config
// load happens here), so callers build it once at startup.
func NewAWSSM(ctx context.Context, region string) (*AWSSM, error) {
	var loadOpts []func(*awsconfig.LoadOptions) error
	if region != "" {
		loadOpts = append(loadOpts, awsconfig.WithRegion(region))
	}
	cfg, err := awsconfig.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, &errs.Error{Op: "secrets.awssm.init", Code: errs.CodeSystem, Cause: err, Hint: "could not load AWS config for Secrets Manager"}
	}
	return &AWSSM{client: secretsmanager.NewFromConfig(cfg)}, nil
}

func (AWSSM) Scheme() string { return "awssm" }

func (a *AWSSM) Resolve(ref string) (string, error) {
	rest := strings.TrimPrefix(ref, "awssm://")
	if rest == "" || rest == ref {
		return "", &errs.Error{
			Op:    "secrets.awssm.resolve",
			Code:  errs.CodeUser,
			Cause: errors.New("awssm: ref missing secret id"),
			Hint:  "use awssm://<secret-id> or awssm://<secret-id>#<json-key>",
		}
	}
	secretID, jsonKey := rest, ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		secretID, jsonKey = rest[:i], rest[i+1:]
	}
	if secretID == "" {
		return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeUser, Cause: errors.New("awssm: empty secret id"), Hint: "use awssm://<secret-id>"}
	}

	out, err := a.client.GetSecretValue(context.Background(), &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretID),
	})
	if err != nil {
		var nf *smtypes.ResourceNotFoundException
		if errors.As(err, &nf) {
			return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeUser, Cause: errs.ErrSecretUnresolved, Hint: "no Secrets Manager secret " + secretID}
		}
		return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeSystem, Cause: err, Hint: "could not read Secrets Manager secret " + secretID}
	}
	if out.SecretString == nil {
		return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeUser, Cause: errs.ErrSecretUnresolved, Hint: "secret " + secretID + " has no string value (binary secrets are not supported)"}
	}
	secret := aws.ToString(out.SecretString)

	if jsonKey == "" {
		return secret, nil
	}
	// Extract one field from a JSON secret.
	var obj map[string]any
	if err := json.Unmarshal([]byte(secret), &obj); err != nil {
		return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeUser, Cause: err, Hint: "secret " + secretID + " is not JSON but a #key was requested"}
	}
	v, ok := obj[jsonKey]
	if !ok {
		return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeUser, Cause: errs.ErrSecretUnresolved, Hint: "secret " + secretID + " has no field " + jsonKey}
	}
	s, ok := v.(string)
	if !ok {
		return "", &errs.Error{Op: "secrets.awssm.resolve", Code: errs.CodeUser, Cause: errors.New("awssm: field is not a string"), Hint: "field " + jsonKey + " of secret " + secretID + " is not a string"}
	}
	return s, nil
}
