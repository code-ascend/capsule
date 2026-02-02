# capsule

Инструмент для создания портативных Linux-контейнеров из OCI-образов. Результат - "ELF-бинарник", использующий C-launcher.

## Особенности
- **Один файл** — скачал и запустил, нет зависимостей
- **Портативность** — можно положить куда угодно, например на USB-флеш-накопитель
- **Без root** — для запуска капсулы не нужны права суперпользователя
- **Бесшовный опыт** — приложения читают и хранят конфиги в вашем `$HOME`
- **Без потери производительности** — это контейнер, не эмуляция
- **Чистая система** — приложение работает в своём окружении со своими библиотеками, не засоряет хост
- **Набор приложений** — одна капсула - целый набор приложений и утилит

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

## Планы
- [ ] Автономное обновление капсул
- [ ] Адаптация утилит и бинарных зависимостей капсулы под ALT Linux

# Credit
- [Conty](https://github.com/Kron4ek/Conty) - For the idea
- [Epm](https://github.com/Etersoft/eepm) - For packages
- [Stplr](https://altlinux.space/stapler/stplr) - For packages
