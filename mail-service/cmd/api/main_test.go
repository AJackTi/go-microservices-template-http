package main

import "testing"

func TestCreateMailReturnsConfiguredMailer(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "localhost")
	t.Setenv("MAIL_HOST", "mailpit")
	t.Setenv("MAIL_PORT", "1025")
	t.Setenv("MAIL_ENCRYPTION", "none")
	t.Setenv("MAIL_USERNAME", "")
	t.Setenv("MAIL_PASSWORD", "")
	t.Setenv("FROM_NAME", "Microservices Lab")
	t.Setenv("FROM_ADDRESS", "no-reply@example.test")

	mailer, err := createMail()
	if err != nil {
		t.Fatalf("createMail returned error: %v", err)
	}

	if mailer.Host != "mailpit" || mailer.Port != 1025 || mailer.FromAddress != "no-reply@example.test" {
		t.Fatalf("unexpected mailer config: %#v", mailer)
	}
}

func TestCreateMailRejectsInvalidPort(t *testing.T) {
	t.Setenv("MAIL_DOMAIN", "localhost")
	t.Setenv("MAIL_HOST", "mailpit")
	t.Setenv("MAIL_PORT", "not-a-number")
	t.Setenv("MAIL_ENCRYPTION", "none")
	t.Setenv("MAIL_USERNAME", "")
	t.Setenv("MAIL_PASSWORD", "")
	t.Setenv("FROM_NAME", "Microservices Lab")
	t.Setenv("FROM_ADDRESS", "no-reply@example.test")

	if _, err := createMail(); err == nil {
		t.Fatal("expected error")
	}
}
