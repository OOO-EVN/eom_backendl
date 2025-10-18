#!/bin/bash
set -e  

PROJECT_NAME="eomstart"
BINARY_NAME="eomstart"
BUILD_DIR="./build"
CMD_PATH="./cmd/server"
DEPLOY_DIR="/opt/eomstart/build"

echo "🚀 Сборка проекта $PROJECT_NAME..."

mkdir -p "$BUILD_DIR"

echo "📦 Выполняем go mod tidy..."
go mod tidy

echo "🔨 Собираем бинарник в $BUILD_DIR/$BINARY_NAME..."
go build -o "$BUILD_DIR/$BINARY_NAME" "$CMD_PATH"

if [ ! -f "$BUILD_DIR/$BINARY_NAME" ]; then
    echo "❌ Ошибка: бинарник не создан."
    exit 1
fi

chmod +x "$BUILD_DIR/$BINARY_NAME"
echo "✅ Сборка успешна! Бинарник создан: $BUILD_DIR/$BINARY_NAME"

# 🔁 Копируем бинарник на продакшн путь
echo "📦 Копируем бинарник в $DEPLOY_DIR..."
sudo mkdir -p "$DEPLOY_DIR"
sudo cp "$BUILD_DIR/$BINARY_NAME" "$DEPLOY_DIR/$BINARY_NAME"
sudo chmod +x "$DEPLOY_DIR/$BINARY_NAME"

echo "✅ Бинарник обновлён: $DEPLOY_DIR/$BINARY_NAME"

# 🔄 Перезапускаем сервис
if systemctl list-units --full -all | grep -Fq "eomstart.service"; then
    echo "♻️ Перезапуск сервиса eomstart..."
    sudo systemctl daemon-reload
    sudo systemctl restart eomstart
    echo "✅ Сервис перезапущен!"
else
    echo "⚠️ Сервис eomstart не найден. Можно создать /etc/systemd/system/eomstart.service"
fi

echo "✨ Готово!"
