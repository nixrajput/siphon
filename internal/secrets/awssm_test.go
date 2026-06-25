package secrets

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"

	"github.com/nixrajput/siphon/internal/errs"
)

// fakeSM returns canned secrets keyed by SecretId, or ResourceNotFound.
type fakeSM struct{ values map[string]string }

func (f fakeSM) GetSecretValue(_ context.Context, in *secretsmanager.GetSecretValueInput, _ ...func(*secretsmanager.Options)) (*secretsmanager.GetSecretValueOutput, error) {
	v, ok := f.values[aws.ToString(in.SecretId)]
	if !ok {
		return nil, &smtypes.ResourceNotFoundException{}
	}
	return &secretsmanager.GetSecretValueOutput{SecretString: aws.String(v)}, nil
}

func newAWSSM(values map[string]string) *AWSSM { return &AWSSM{client: fakeSM{values: values}} }

func TestAWSSM_WholeSecret(t *testing.T) {
	a := newAWSSM(map[string]string{"prod/db": "plain-password"})
	if got, err := a.Resolve("awssm://prod/db"); err != nil || got != "plain-password" {
		t.Errorf("whole secret = (%q, %v), want (plain-password, nil)", got, err)
	}
}

func TestAWSSM_JSONFieldSelector(t *testing.T) {
	a := newAWSSM(map[string]string{"prod/db": `{"username":"app","password":"hunter2"}`})
	if got, err := a.Resolve("awssm://prod/db#password"); err != nil || got != "hunter2" {
		t.Errorf("#password = (%q, %v), want (hunter2, nil)", got, err)
	}
}

func TestAWSSM_Errors(t *testing.T) {
	a := newAWSSM(map[string]string{"prod/db": `{"password":"x"}`})
	cases := map[string]string{
		"not found":          "awssm://nope",
		"missing field":      "awssm://prod/db#username",
		"non-json with #key": "awssm://lit#k", // lit isn't seeded → not found actually; see below
		"empty ref":          "awssm://",
		"empty # selector":   "awssm://prod/db#", // must reject, not return whole secret
	}
	// Seed a non-JSON secret so the #key-on-non-json path is exercised distinctly.
	a.client = fakeSM{values: map[string]string{"prod/db": `{"password":"x"}`, "lit": "not-json"}}

	for name, ref := range cases {
		_, err := a.Resolve(ref)
		var e *errs.Error
		if !errors.As(err, &e) || e.Code != errs.CodeUser {
			t.Errorf("%s: Resolve(%q) err = %v, want CodeUser", name, ref, err)
		}
	}
}

func TestAWSSM_Scheme(t *testing.T) {
	if (AWSSM{}).Scheme() != "awssm" {
		t.Errorf("Scheme() = %q, want awssm", (AWSSM{}).Scheme())
	}
}
