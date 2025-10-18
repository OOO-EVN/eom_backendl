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

# 🔁 Копируем бинарник на продакшн путь безопасно
echo "📦 Копируем бинарник в $DEPLOY_DIR..."
sudo mkdir -p "$DEPLOY_DIR"

# Копируем во временный файл
TMP_BINARY="$DEPLOY_DIR/$BINARY_NAME.tmp"
sudo cp "$BUILD_DIR/$BINARY_NAME" "$TMP_BINARY"
sudo chmod +x "$TMP_BINARY"

# Атомарно заменяем старый бинарник
sudo mv "$TMP_BINARY" "$DEPLOY_DIR/$BINARY_NAME"
echo "✅ Бинарник обновлён: $DEPLOY_DIR/$BINARY_NAME"

# 🔄 Перезапускаем сервис безопасно
if systemctl list-units --full -all | grep -Fq "eomstart.service"; then
    echo "♻️ Перезапуск сервиса eomstart..."
    sudo systemctl daemon-reload
    sudo systemctl restart eomstart
    echo "✅ Сервис перезапущен!"
else
    echo "⚠️ Сервис eomstart не найден. Можно создать /etc/systemd/system/eomstart.service"
fi

echo "✨ Готово!"
