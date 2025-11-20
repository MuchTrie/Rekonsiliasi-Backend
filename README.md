# Switching Reconciliation Backend

Backend API untuk Sistem Rekonsiliasi Switching yang dibangun menggunakan Go (Golang) dengan framework Gin dan GORM untuk manajemen database MySQL.

## 📋 Deskripsi

Aplikasi ini menyediakan REST API untuk proses rekonsiliasi data switching antara sistem CORE dengan file rekonsiliasi dan settlement dari vendor (ALTO, JALIN). Fitur utama meliputi:

- ✅ **Proses Rekonsiliasi** - Membandingkan data CORE dengan file rekonsiliasi vendor
- ✅ **Settlement Converter** - Konversi format file settlement ke CSV
- ✅ **Riwayat Rekonsiliasi** - Melihat hasil rekonsiliasi sebelumnya
- ✅ **Autentikasi & Autorisasi** - JWT-based authentication dengan role-based access control
- ✅ **Settings Management** - Toggle fitur untuk role operasional

## 🛠️ Tech Stack

- **Language**: Go 1.24+
- **Framework**: Gin v1.10.0
- **ORM**: GORM
- **Database**: MySQL 8.0+
- **Auth**: JWT (golang-jwt/jwt v5)
- **Logging**: Logrus
- **Environment**: godotenv

## 📦 Prerequisites

Pastikan sistem sudah terinstal:

- [Go](https://go.dev/dl/) versi 1.24 atau lebih baru
- [MySQL](https://dev.mysql.com/downloads/mysql/) versi 8.0 atau lebih baru
- Git (untuk clone repository)

Verifikasi instalasi:
```bash
go version
mysql --version
```

## 🚀 Instalasi

### 1. Clone Repository

```bash
git clone <repository-url>
cd backend
```

### 2. Install Dependencies

```bash
go mod download
go mod tidy
```

### 3. Setup Database

Buat database MySQL baru:

```sql
CREATE DATABASE switching_reconcile_db CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

### 4. Konfigurasi Environment

Copy file `.env.example` menjadi `.env`:

```bash
cp .env.example .env
```

Edit file `.env` sesuai konfigurasi Anda:

```env
# Database Configuration
DB_HOST=localhost
DB_PORT=3306
DB_USER=root
DB_PASSWORD=your_mysql_password
DB_NAME=switching_reconcile_db

# JWT Configuration
JWT_SECRET=your-super-secret-key-change-this-in-production-min-32-chars

# Server Configuration
PORT=8080
GIN_MODE=debug  

### 5. Build Aplikasi

```bash
go build -o bin/server.exe cmd/server/main.go
```

Atau untuk Linux/Mac:
```bash
go build -o bin/server cmd/server/main.go
```

## ▶️ Menjalankan Aplikasi

### Development Mode

```bash
go run cmd/server/main.go
```

### Production Mode (Compiled Binary)

Windows:
```bash
.\bin\server.exe
```

Linux/Mac:
```bash
./bin/server
```

Server akan berjalan di `http://localhost:8080`

### Auto-Migration & Seeder

Aplikasi akan otomatis:
1. **Membuat tabel** yang diperlukan (users, settings) saat startup
2. **Menjalankan seeder** untuk membuat user default dan pengaturan awal

**Default Users** yang dibuat:

| Username | Password | Role | Email |
|----------|----------|------|-------|
| admin@switching.com | admin123 | admin | admin@example.com |
| operasional@switching.com | operasional123 | operasional | ops@example.com |



## 📁 Struktur Direktori

```
backend/
├── auth/                      # Modul autentikasi & autorisasi
│   ├── database/             # Database connection
│   ├── handler/              # Auth & Settings HTTP handlers
│   ├── middleware/           # JWT authentication middleware
│   ├── models/               # User & Settings models
│   └── seeder/               # Database seeders
├── cmd/
│   └── server/               # Entry point aplikasi
│       └── main.go
├── internal/                 # Business logic
│   ├── dto/                  # Data Transfer Objects
│   ├── handler/              # Reconciliation handlers
│   ├── middleware/           # CORS middleware
│   └── service/              # Core services
│       ├── csv_helper.go           # CSV utilities
│       ├── data_extractor.go       # Data extraction
│       ├── file_converter.go       # Format converter
│       ├── reconciliation_comparator.go
│       ├── reconciliation_service.go
│       └── settlement_converter.go
├── pkg/
│   └── validator/            # File validation
├── results/                  # Hasil rekonsiliasi (auto-generated)
├── uploads/                  # File upload temporary
│   └── temp/
├── .env                      # Environment configuration
├── .env.example              # Template environment
├── go.mod                    # Go dependencies
└── README.md                 # Dokumentasi ini
```

## 🔌 API Endpoints

### Health Check

```http
GET /api/health
```

### Authentication

```http
POST /api/auth/login
Content-Type: application/json

{
  "username": "admin",
  "password": "admin123"
}
```

Response:
```json
{
  "success": true,
  "message": "Login successful",
  "data": {
    "user": {
      "id": 1,
      "username": "admin",
      "email": "admin@example.com",
      "role": "admin"
    },
    "token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
  }
}
```

### Protected Endpoints (Require Authentication)

Semua endpoint berikut memerlukan header:
```
Authorization: Bearer <your-jwt-token>
```


## 🔐 Role-Based Access Control

| Endpoint | Admin | Operasional |
|----------|-------|-------------|
| `/api/auth/login` | ✅ | ✅ |
| `/api/reconciliation/process` | ✅ | ✅ (jika enabled) |
| `/api/convert/settlement` | ✅ | ✅ (jika enabled) |
| `/api/results` | ✅ | ✅ (jika enabled) |
| `/api/settings` | ✅ | ❌ |

Admin dapat mengaktifkan/menonaktifkan fitur untuk role operasional melalui settings.

### JWT Token Invalid

```
Error: invalid or expired jwt
```

**Solusi**:
- Login ulang untuk mendapatkan token baru
- Pastikan `JWT_SECRET` tidak berubah
- Token expired setelah 24 jam (default)


### Testing dengan Postman

1. Import collection dari dokumentasi API
2. Set environment variable `baseUrl` = `http://localhost:8080`
3. Set authorization token setelah login
4. Test setiap endpoint sesuai kebutuhan

## 📝 Logging

Aplikasi menggunakan **Logrus** untuk logging dengan format JSON.

Log levels:
- `INFO` - Informasi umum (startup, request success)
- `WARN` - Peringatan (deprecated features)
- `ERROR` - Error yang di-handle
- `FATAL` - Critical error (app terminate)




## 📞 Support

Jika mengalami masalah atau ada pertanyaan:

1. Check dokumentasi ini terlebih dahulu
2. Review error logs di console
3. Pastikan semua prerequisites terinstall
4. Verifikasi konfigurasi `.env` sudah benar



## 👥 Contributors

- Developer Team - Much & Wibi

---

**Last Updated**: November 20, 2025  
**Version**: 1.0.0
