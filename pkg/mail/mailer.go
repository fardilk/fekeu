package mail

import (
	"log"
)

// SendOTP is a dummy mailer that logs the OTP to server logs.
// Replace with a real SMTP/API integration later.
func SendOTP(email, otp string) error {
	log.Printf("[MAIL] Sending OTP %s to %s", otp, email)
	return nil
}
