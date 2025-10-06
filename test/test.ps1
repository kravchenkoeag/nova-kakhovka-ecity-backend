# Цвета для вывода
$Green = 'Green'
$Red = 'Red'
$Yellow = 'Yellow'
$Cyan = 'Cyan'

# Базовый URL
$baseUrl = "http://localhost:8080"

Write-Host "`n🚀 Начинаем тестирование API...`n" -ForegroundColor $Cyan

# 1. Проверка здоровья
Write-Host "1️⃣  Проверка /health..." -ForegroundColor $Yellow
try {
    $response = curl.exe -s "$baseUrl/health"
    Write-Host "✅ $response" -ForegroundColor $Green
} catch {
    Write-Host "❌ Сервер не отвечает!" -ForegroundColor $Red
    exit
}

# 2. Регистрация
Write-Host "`n2️⃣  Регистрация пользователя..." -ForegroundColor $Yellow
$registerJson = @'
{
  "email": "test@example.com",
  "password": "password123",
  "first_name": "Тест",
  "last_name": "Пользователь"
}
'@

$registerJson | Out-File -FilePath "temp_register.json" -Encoding UTF8

$response = curl.exe -X POST "$baseUrl/api/v1/register" `
    -H "Content-Type: application/json" `
    -d "@temp_register.json" `
    -s

Write-Host $response -ForegroundColor $Green

# 3. Вход
Write-Host "`n3️⃣  Вход в систему..." -ForegroundColor $Yellow
$loginJson = @'
{
  "email": "test@example.com",
  "password": "password123"
}
'@

$loginJson | Out-File -FilePath "temp_login.json" -Encoding UTF8

$response = curl.exe -X POST "$baseUrl/api/v1/login" `
    -H "Content-Type: application/json" `
    -d "@temp_login.json" `
    -s

Write-Host $response -ForegroundColor $Green

# Извлекаем токен
$token = ($response | ConvertFrom-Json).token

if ($token) {
    Write-Host "`n🎫 Token получен: $token" -ForegroundColor $Cyan
    
    # 4. Получение профиля
    Write-Host "`n4️⃣  Получение профиля..." -ForegroundColor $Yellow
    $response = curl.exe -X GET "$baseUrl/api/v1/profile" `
        -H "Authorization: Bearer $token" `
        -s
    
    Write-Host $response -ForegroundColor $Green
}

# Очистка временных файлов
Remove-Item "temp_register.json" -ErrorAction SilentlyContinue
Remove-Item "temp_login.json" -ErrorAction SilentlyContinue

Write-Host "`n✨ Тестирование завершено!`n" -ForegroundColor $Cyan