# API Test Script for Nova Kakhovka eCity Backend
# Test data will be recorded in sum_test.txt

$baseUrl = "http://localhost:8080"
$apiVersion = "v1"

# Test data
$testData = @{
    email = "testuser_$(Get-Date -Format 'yyyyMMddHHmmss')@example.com"
    password = "TestPassword123!"
    firstName = "Test"
    lastName = "User"
    groupName = "Test Group $(Get-Date -Format 'HHmmss')"
    groupDescription = "A test group for API testing"
}

$results = @()
$token = ""
$userId = ""
$groupId = ""

# Function to make API requests
function Test-API {
    param(
        [string]$Method,
        [string]$Endpoint,
        [hashtable]$Headers = @{},
        [string]$Body = $null,
        [string]$TestName
    )
    
    try {
        $headersObj = @{}
        if ($Headers) {
            foreach ($key in $Headers.Keys) {
                $headersObj[$key] = $Headers[$key]
            }
        }
        
        $params = @{
            Uri = "$baseUrl$Endpoint"
            Method = $Method
            Headers = $headersObj
            ContentType = "application/json"
            UseBasicParsing = $true
            ErrorAction = "Stop"
        }
        
        if ($Body) {
            $params.Body = $Body
        }
        
        $response = Invoke-WebRequest @params
        $statusCode = $response.StatusCode
        $responseBody = $response.Content | ConvertFrom-Json
        
        return @{
            Success = $true
            StatusCode = $statusCode
            Response = $responseBody
            Error = $null
        }
    }
    catch {
        $statusCode = $_.Exception.Response.StatusCode.value__
        $errorMsg = $_.Exception.Message
        try {
            $reader = New-Object System.IO.StreamReader($_.Exception.Response.GetResponseStream())
            $responseBody = $reader.ReadToEnd() | ConvertFrom-Json
            $errorMsg = $responseBody.error
        } catch {
            # If we can't parse the error, use the exception message
        }
        
        return @{
            Success = $false
            StatusCode = $statusCode
            Response = $null
            Error = $errorMsg
        }
    }
}

Write-Host "=== Starting API Tests ===" -ForegroundColor Green
Write-Host "Test Data:" -ForegroundColor Yellow
Write-Host "  Email: $($testData.email)"
Write-Host "  Password: $($testData.password)"
Write-Host "  Name: $($testData.firstName) $($testData.lastName)"
Write-Host ""

# Test 1: Health Check
Write-Host "Test 1: Health Check" -ForegroundColor Cyan
$result = Test-API -Method "GET" -Endpoint "/health" -TestName "Health Check"
$results += "Health Check: Status $($result.StatusCode) - $($result.Success)"
Write-Host "  Status: $($result.StatusCode)" -ForegroundColor $(if ($result.Success) { "Green" } else { "Red" })
if ($result.Error) { Write-Host "  Error: $($result.Error)" -ForegroundColor Red }
Write-Host ""

# Test 2: Register User
Write-Host "Test 2: Register User" -ForegroundColor Cyan
$registerBody = @{
    email = $testData.email
    password = $testData.password
    first_name = $testData.firstName
    last_name = $testData.lastName
} | ConvertTo-Json

$result = Test-API -Method "POST" -Endpoint "/api/$apiVersion/auth/register" -Body $registerBody -TestName "Register User"
$results += "Register User: Status $($result.StatusCode) - $($result.Success)"
Write-Host "  Status: $($result.StatusCode)" -ForegroundColor $(if ($result.Success) { "Green" } else { "Red" })
if ($result.Success -and $result.Response.id) {
    $userId = $result.Response.id
    Write-Host "  User ID: $userId" -ForegroundColor Green
}
if ($result.Error) { Write-Host "  Error: $($result.Error)" -ForegroundColor Red }
Write-Host ""

# Test 3: Login
Write-Host "Test 3: Login" -ForegroundColor Cyan
$loginBody = @{
    email = $testData.email
    password = $testData.password
} | ConvertTo-Json

$result = Test-API -Method "POST" -Endpoint "/api/$apiVersion/auth/login" -Body $loginBody -TestName "Login"
$results += "Login: Status $($result.StatusCode) - $($result.Success)"
Write-Host "  Status: $($result.StatusCode)" -ForegroundColor $(if ($result.Success) { "Green" } else { "Red" })
if ($result.Success -and $result.Response.token) {
    $token = $result.Response.token
    Write-Host "  Token: $($token.Substring(0, [Math]::Min(50, $token.Length)))..." -ForegroundColor Green
}
if ($result.Error) { Write-Host "  Error: $($result.Error)" -ForegroundColor Red }
Write-Host ""

# Test 4: Get Profile (requires auth)
if ($token) {
    Write-Host "Test 4: Get Profile" -ForegroundColor Cyan
    $headers = @{
        "Authorization" = "Bearer $token"
    }
    $result = Test-API -Method "GET" -Endpoint "/api/$apiVersion/auth/profile" -Headers $headers -TestName "Get Profile"
    $results += "Get Profile: Status $($result.StatusCode) - $($result.Success)"
    Write-Host "  Status: $($result.StatusCode)" -ForegroundColor $(if ($result.Success) { "Green" } else { "Red" })
    if ($result.Error) { Write-Host "  Error: $($result.Error)" -ForegroundColor Red }
    Write-Host ""
}

# Test 5: Create Group (requires auth)
if ($token) {
    Write-Host "Test 5: Create Group" -ForegroundColor Cyan
    $groupBody = @{
        name = $testData.groupName
        description = $testData.groupDescription
        type = "interest"
        is_public = $true
        auto_join = $false
        max_members = 100
    } | ConvertTo-Json

    $headers = @{
        "Authorization" = "Bearer $token"
    }
    $result = Test-API -Method "POST" -Endpoint "/api/$apiVersion/groups" -Headers $headers -Body $groupBody -TestName "Create Group"
    $results += "Create Group: Status $($result.StatusCode) - $($result.Success)"
    Write-Host "  Status: $($result.StatusCode)" -ForegroundColor $(if ($result.Success) { "Green" } else { "Red" })
    if ($result.Success -and $result.Response.id) {
        $groupId = $result.Response.id
        Write-Host "  Group ID: $groupId" -ForegroundColor Green
    }
    if ($result.Error) { Write-Host "  Error: $($result.Error)" -ForegroundColor Red }
    Write-Host ""
}

# Test 6: Get Groups (requires auth)
if ($token) {
    Write-Host "Test 6: Get Groups" -ForegroundColor Cyan
    $headers = @{
        "Authorization" = "Bearer $token"
    }
    $result = Test-API -Method "GET" -Endpoint "/api/$apiVersion/groups" -Headers $headers -TestName "Get Groups"
    $results += "Get Groups: Status $($result.StatusCode) - $($result.Success)"
    Write-Host "  Status: $($result.StatusCode)" -ForegroundColor $(if ($result.Success) { "Green" } else { "Red" })
    if ($result.Error) { Write-Host "  Error: $($result.Error)" -ForegroundColor Red }
    Write-Host ""
}

Write-Host "=== Test Summary ===" -ForegroundColor Green
$results | ForEach-Object { Write-Host $_ }

# Write test data to file
$summary = @"
=== Nova Kakhovka eCity API Test Data ===
Test Date: $(Get-Date -Format 'yyyy-MM-dd HH:mm:ss')

TEST DATA USED:
---------------
Email: $($testData.email)
Password: $($testData.password)
First Name: $($testData.firstName)
Last Name: $($testData.lastName)
Group Name: $($testData.groupName)
Group Description: $($testData.groupDescription)

GENERATED IDs:
--------------
User ID: $userId
Group ID: $groupId
Token: $(if ($token) { "$($token.Substring(0, [Math]::Min(50, $token.Length)))..." } else { "Not obtained" })

TEST RESULTS:
-------------
$($results -join "`n")

ENDPOINTS TESTED:
-----------------
1. GET  /health
2. POST /api/$apiVersion/auth/register
3. POST /api/$apiVersion/auth/login
4. GET  /api/$apiVersion/auth/profile
5. POST /api/$apiVersion/groups
6. GET  /api/$apiVersion/groups

"@

$summary | Out-File -FilePath "sum_test.txt" -Encoding UTF8
Write-Host "`nTest data recorded in sum_test.txt" -ForegroundColor Green
