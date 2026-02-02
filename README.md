# capsule

Инструмент для создания портативных Linux-контейнеров из OCI-образов. Результат - "ELF-бинарник", использующий C-launcher.

## Установка

```bash
sudo apt-get install glibc-devel-static gcc14 gcc squashfs-tools go
go build -o ./build/capsule ./cmd/capsule
./build/capsule -h
```

## Пример сборки портативной капсулы
```bash
# Сборка google-chrome
sudo ./build/capsule build ./examples/chrome.yaml -v
# Справка
chrome -h
# Запуск 
chrome
# Войти внутрь, все изменения будут сохранены
chrome --shell
# Экспорт ярлыков
chrome --export
# Убрать ярлыки из хост системы
chrome --unexport
```

# Credit
- [Conty](https://github.com/Kron4ek/Conty) - For the idea
- [Epm](https://github.com/Etersoft/eepm) - For packages
- [Stplr](https://altlinux.space/stapler/stplr) - For packages
