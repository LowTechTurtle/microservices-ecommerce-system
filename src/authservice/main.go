package main

import (
    "authservice/store"
    "context"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "time"

    "github.com/aws/aws-lambda-go/events"
    "github.com/aws/aws-lambda-go/lambda"
    "github.com/golang-jwt/jwt/v5"
    "golang.org/x/oauth2"
    "golang.org/x/oauth2/google"
)

var (
    userStore    store.UserStore
    oauth2Config *oauth2.Config
    jwtSecret    string
)

func init() {
    tableName := os.Getenv("DYNAMODB_TABLE_NAME")
    if tableName == "" {
        tableName = "Users"
    }

    var err error
    userStore, err = store.NewDynamoUserStore(context.Background(), tableName)
    if err != nil {
        log.Fatalf("Failed to initialize DynamoDB: %v", err)
    }

    // Google OAuth config
    oauth2Config = &oauth2.Config{
        ClientID:     os.Getenv("GOOGLE_CLIENT_ID"),
        ClientSecret: os.Getenv("GOOGLE_CLIENT_SECRET"),
        RedirectURL:  os.Getenv("GOOGLE_REDIRECT_URL"),
        Scopes: []string{
            "https://www.googleapis.com/auth/userinfo.email",
            "https://www.googleapis.com/auth/userinfo.profile",
        },
        Endpoint: google.Endpoint,
    }

    jwtSecret = os.Getenv("JWT_SECRET")
    if jwtSecret == "" {
        jwtSecret = "default-secret-change-in-production"
    }
}

func respondJSON(statusCode int, body interface{}) (events.APIGatewayV2HTTPResponse, error) {
    b, _ := json.Marshal(body)
    return events.APIGatewayV2HTTPResponse{
        StatusCode: statusCode,
        Headers: map[string]string{
            "Content-Type":                "application/json",
            "Access-Control-Allow-Origin": "*",
        },
        Body: string(b),
    }, nil
}

func HandleRequest(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
    method := req.RequestContext.HTTP.Method
    path := req.RawPath

    // Dynamically set redirect URL if not provided in env vars
    if oauth2Config.RedirectURL == "" && req.RequestContext.DomainName != "" {
        oauth2Config.RedirectURL = "https://" + req.RequestContext.DomainName + "/auth/callback"
    }

    // Handle CORS preflight
    if method == "OPTIONS" {
        return events.APIGatewayV2HTTPResponse{
            StatusCode: 200,
            Headers: map[string]string{
                "Access-Control-Allow-Origin":  "*",
                "Access-Control-Allow-Methods": "GET,POST,OPTIONS",
                "Access-Control-Allow-Headers": "Content-Type,Authorization",
            },
        }, nil
    }

    // GET /auth/google - Start OAuth flow
    if method == "GET" && path == "/auth/google" {
        return handleGoogleLogin(ctx, req)
    }

    // GET /auth/callback - OAuth callback
    if method == "GET" && path == "/auth/callback" {
        return handleGoogleCallback(ctx, req)
    }

    // GET /auth/verify - Verify JWT token
    if method == "GET" && path == "/auth/verify" {
        return handleVerifyToken(ctx, req)
    }

    // GET /auth/user - Get user info
    if method == "GET" && path == "/auth/user" {
        return handleGetUser(ctx, req)
    }

    return respondJSON(http.StatusNotFound, map[string]string{"error": "not found"})
}

func handleGoogleLogin(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
    state := req.QueryStringParameters["state"]
    if state == "" {
        state = "random-state"
    }
    
    url := oauth2Config.AuthCodeURL(state, oauth2.AccessTypeOffline)
    
    return events.APIGatewayV2HTTPResponse{
        StatusCode: 302,
        Headers: map[string]string{
            "Location": url,
        },
    }, nil
}

func handleGoogleCallback(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
    code := req.QueryStringParameters["code"]
    if code == "" {
        return respondJSON(http.StatusBadRequest, map[string]string{"error": "code is required"})
    }

    // Exchange code for token
    token, err := oauth2Config.Exchange(ctx, code)
    if err != nil {
        return respondJSON(http.StatusBadRequest, map[string]string{"error": "failed to exchange token"})
    }

    // Get user info from Google
    client := oauth2Config.Client(ctx, token)
    resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
    if err != nil {
        return respondJSON(http.StatusInternalServerError, map[string]string{"error": "failed to get user info"})
    }
    defer resp.Body.Close()

    var googleUser struct {
        ID      string `json:"id"`
        Email   string `json:"email"`
        Name    string `json:"name"`
        Picture string `json:"picture"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&googleUser); err != nil {
        return respondJSON(http.StatusInternalServerError, map[string]string{"error": "failed to decode user info"})
    }

    // Check if user exists
    user, err := userStore.GetUserByGoogleID(ctx, googleUser.ID)
    if err != nil {
        return respondJSON(http.StatusInternalServerError, map[string]string{"error": "database error"})
    }

    // Create new user if not exists
    if user == nil {
        user, err = userStore.CreateUser(ctx, googleUser.ID, googleUser.Email, googleUser.Name, googleUser.Picture)
        if err != nil {
            return respondJSON(http.StatusInternalServerError, map[string]string{"error": "failed to create user"})
        }
    } else {
        // Update last login
        userStore.UpdateLastLogin(ctx, user.UserID)
    }

    // Generate JWT token
    jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
        "user_id": user.UserID,
        "email":   user.Email,
        "exp":     time.Now().Add(24 * time.Hour).Unix(),
    })

    tokenString, err := jwtToken.SignedString([]byte(jwtSecret))
    if err != nil {
        return respondJSON(http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
    }

    frontendURL := os.Getenv("FRONTEND_URL")
    if frontendURL == "" {
        frontendURL = "http://localhost:8080"
    }

    redirectURL := fmt.Sprintf("%s/login/callback?token=%s", frontendURL, tokenString)

    return events.APIGatewayV2HTTPResponse{
        StatusCode: 302,
        Headers: map[string]string{
            "Location": redirectURL,
        },
    }, nil
}

func handleVerifyToken(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
    authHeader := req.Headers["authorization"]
    if authHeader == "" {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "authorization header required"})
    }

    tokenString := authHeader
    if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
        tokenString = authHeader[7:]
    }

    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
            return nil, fmt.Errorf("unexpected signing method")
        }
        return []byte(jwtSecret), nil
    })

    if err != nil || !token.Valid {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
    }

    return respondJSON(http.StatusOK, map[string]interface{}{
        "valid":   true,
        "user_id": claims["user_id"],
        "email":   claims["email"],
    })
}

func handleGetUser(ctx context.Context, req events.APIGatewayV2HTTPRequest) (events.APIGatewayV2HTTPResponse, error) {
    authHeader := req.Headers["authorization"]
    if authHeader == "" {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "authorization header required"})
    }

    tokenString := authHeader
    if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
        tokenString = authHeader[7:]
    }

    token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
        return []byte(jwtSecret), nil
    })

    if err != nil || !token.Valid {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "invalid token"})
    }

    claims, ok := token.Claims.(jwt.MapClaims)
    if !ok {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "invalid token claims"})
    }

    userID, ok := claims["user_id"].(string)
    if !ok {
        return respondJSON(http.StatusUnauthorized, map[string]string{"error": "invalid user_id in token"})
    }

    user, err := userStore.GetUserByID(ctx, userID)
    if err != nil || user == nil {
        return respondJSON(http.StatusNotFound, map[string]string{"error": "user not found"})
    }

    return respondJSON(http.StatusOK, user)
}

func main() {
    lambda.Start(HandleRequest)
}
