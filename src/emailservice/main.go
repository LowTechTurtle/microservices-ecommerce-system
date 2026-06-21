package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/aws/aws-lambda-go/lambda"
	"github.com/joho/godotenv"
)

type EmailRequest struct {
	Email       string `json:"email"`
	OrderID     string `json:"order_id"`
	PaymentCode string `json:"payment_code"`
	Total       string `json:"total"`
}

func respondJSON(status int, body interface{}) (events.APIGatewayV2HTTPResponse, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return events.APIGatewayV2HTTPResponse{StatusCode: 500, Body: `{"error":"marshal error"}`}, nil
	}
	return events.APIGatewayV2HTTPResponse{
		StatusCode: status,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(b),
	}, nil
}

func HandleRequest(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
	if req.RequestContext.HTTP.Method != "POST" {
		return respondJSON(http.StatusMethodNotAllowed, map[string]string{"error": "Method Not Allowed"})
	}

	var emailReq EmailRequest
	if err := json.Unmarshal([]byte(req.Body), &emailReq); err != nil {
		log.Printf("Failed to unmarshal body: %v", err)
		return respondJSON(http.StatusBadRequest, map[string]string{"error": "Invalid request body"})
	}

	if emailReq.Email == "" || emailReq.OrderID == "" {
		return respondJSON(http.StatusBadRequest, map[string]string{"error": "Missing email or order_id"})
	}

	smtpHost := os.Getenv("SMTP_HOST")
	smtpPort := os.Getenv("SMTP_PORT")
	smtpUser := os.Getenv("SMTP_USERNAME")
	smtpPass := os.Getenv("SMTP_PASSWORD")

	if smtpHost == "" || smtpUser == "" || smtpPass == "" {
		log.Println("SMTP credentials not fully configured")
		return respondJSON(http.StatusInternalServerError, map[string]string{"error": "SMTP config error"})
	}

	auth := smtp.PlainAuth("", smtpUser, smtpPass, smtpHost)

	subject := fmt.Sprintf("Order Confirmation - %s", emailReq.OrderID)

	var body bytes.Buffer
	body.WriteString("MIME-version: 1.0;\nContent-Type: text/html; charset=\"UTF-8\";\n\n")
	body.WriteString(fmt.Sprintf(`
		<html>
		<body style="font-family: Arial, sans-serif; color: #333; line-height: 1.6; max-width: 600px; margin: 0 auto; padding: 20px;">
			<h2 style="color: #4285F4;">Cảm ơn bạn đã đặt hàng!</h2>
			<p>Đơn hàng <strong>%s</strong> của bạn đã được tiếp nhận.</p>
			
			<div style="background-color: #f9f9f9; padding: 15px; border-radius: 8px; margin: 20px 0;">
				<h3 style="margin-top: 0; color: #333;">Chi tiết thanh toán</h3>
				<p>Tổng tiền: <strong>%s</strong></p>
				<p>Mã thanh toán: <strong style="font-size: 1.2em; color: #E53935; letter-spacing: 2px;">%s</strong></p>
			</div>
			
			<p>Vui lòng sử dụng mã thanh toán trên khi chuyển khoản để chúng tôi xác nhận nhanh nhất.</p>
			<p>Nếu bạn có bất kỳ câu hỏi nào, vui lòng trả lời email này.</p>
			
			<br/>
			<p>Trân trọng,<br/><strong>Đội ngũ Microservices Ecommerce</strong></p>
		</body>
		</html>
	`, emailReq.OrderID, emailReq.Total, emailReq.PaymentCode))

	msg := []byte("To: " + emailReq.Email + "\r\n" +
		"Subject: " + subject + "\r\n" +
		body.String())

	err := smtp.SendMail(smtpHost+":"+smtpPort, auth, smtpUser, []string{emailReq.Email}, msg)
	if err != nil {
		log.Printf("Failed to send email to %s: %v", emailReq.Email, err)
		return respondJSON(http.StatusInternalServerError, map[string]string{"error": "Failed to send email"})
	}

	log.Printf("Successfully sent order confirmation to %s", emailReq.Email)
	return respondJSON(http.StatusOK, map[string]string{"message": "Email sent successfully"})
}

func main() {
	// Attempt to load .env file if it exists
	_ = godotenv.Load()
	
	lambda.Start(HandleRequest)
}
