package main

import "testing"

func TestSubmissionRequestValidate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request SubmissionRequest
		wantErr bool
	}{
		{
			name: "auth ok",
			request: SubmissionRequest{
				Action: "auth",
				Auth:   AuthInput{Email: "admin@example.com", Password: "secret"},
			},
		},
		{
			name: "log missing data",
			request: SubmissionRequest{
				Action: "log",
				Log:    LogInput{Name: "event"},
			},
			wantErr: true,
		},
		{
			name: "mail missing subject",
			request: SubmissionRequest{
				Action: "mail",
				Mail:   MailInput{To: "receiver@example.com", Message: "hello"},
			},
			wantErr: true,
		},
		{
			name: "unknown action",
			request: SubmissionRequest{
				Action: "explode",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.request.Validate()
			if tt.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestSubmissionWorkflowBrokerReturnsCanonicalResponse(t *testing.T) {
	t.Parallel()

	result := (&SubmissionWorkflow{}).Broker()

	if result.Status != 200 || result.Payload.Message != "Hit the broker" || result.Payload.Error {
		t.Fatalf("unexpected broker result: %#v", result)
	}
}
