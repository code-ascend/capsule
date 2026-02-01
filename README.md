# capsule

Инструмент для создания портативных Linux-контейнеров из OCI-образов. Результат - "ELF-бинарник", использующий C-launcher.

## Установка

```bash
sudo apt-get install glibc-devel-static gcc14
go build -o ./build/capsule ./cmd/capsule
./build/capsule -h
```

## Сборка контейнера
```bash
sudo ./build/capsule build ./examples/chrome.yaml
```

# Credit
- [Conty](https://github.com/Kron4ek/Conty) - For the idea
- [Epm](https://github.com/Etersoft/eepm) - For packages
- [Stplr](https://altlinux.space/stapler/stplr) - For packages
