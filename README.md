# capsule

Инструмент для создания портативных Linux-контейнеров из OCI-образов. Результат — единый ELF-бинарник со статически собранным Go-рантаймом внутри.

## Особенности
- **Один файл** — скачал и запустил, нет зависимостей
- **Портативность** — можно положить куда угодно, например на USB-флеш-накопитель
- **Без root** — для запуска капсулы не нужны права суперпользователя
- **Бесшовный опыт** — приложения читают и хранят конфиги в вашем `$HOME`
- **Чистая система** — приложение работает в своём окружении со своими библиотеками, не засоряет хост
- **Набор приложений** — одна капсула — целый набор приложений и утилит
- **Статический Go-рантайм** — без зависимости от системной glibc/bash
- **Изолированный рантайм** — утилиты работают через встроенный ld-linux и libc

## Установка
### Установка из ALS
> Сами капсулы обладают большой автономностью и портативностью, но для их сборки требуется данный проект
```bash
apm repo add rpm https://altlinux.space/api/packages/dmitry/alt/group/capsule-nightly/sisyphus.repo _arch_ classic
apm s update
apm s install capsule
```

### Установка вручную
```bash
sudo apt-get install squashfs-tools go \
                     libgpgme-devel libbtrfs-devel libdevmapper-devel \
                     shadow-submap fuse-overlayfs containers-common
# go generate соберёт встраиваемый Go-рантайм через scripts/build-runtime.sh
go generate ./...
go build -o ./build/capsule ./cmd/capsule
./build/capsule -h
```

## Пример сборки портативной капсулы
> Внимание! Это пример сборки google-chrome, в `examples` есть другие примеры с разным набором рантаймов и пакетов.
> Цель данного примера - ознакомление со сборкой капсул. Конечная задача после ознакомления - создать собственную капсулу с нужным набором пакетов.
```bash
# Сборка google-chrome
capsule build ./examples/chrome.yaml -v
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
## Особенности работы с капсулами

Все изменения, сделанные внутри капсулы (установка пакетов, настройка конфигов), сохраняются между запусками и могут быть зафиксированы в образе.

```bash
# Войти в shell, установить новый пакеты, изменить файлы
chrome --shell
# Зафиксировать изменения 
chrome --commit
# Обновить капсулу
sudo chrome --update
# Сбросить все изменения (те изменения что не были зафиксированы через --commit)
chrome --clean
```

## Как это работает

Capsule — это единый исполняемый файл со следующей структурой:

| Go runtime + utils.tar.gz | binconfig (JSON) | SquashFS | Footer  |
|---------------------------|------------------|----------|---------|
| ~8.5 MB                   | сотни байт       | образ    | 32 байт |

## По Пунктам
1. Go-рантайм при запуске читает `/proc/self/exe`, парсит 32-байтовый footer и узнаёт смещения встроенных полезных нагрузок (binconfig + squashfs).
2. Из себя же распаковывает `utils.tar.gz` (bwrap, squashfuse, unionfs, mksquashfs, ld-linux, либы) во временный workspace.
3. Squashfuse монтирует SquashFS-образ прямо из бинарника по смещению (-o offset=N) через FUSE — без root, в userspace.
4. Опционально поверх кладётся записываемый unionfs-overlay, в который сливаются изменения сессии и перенесённые с хоста NVIDIA-библиотеки.
5. Bubblewrap создаёт изолированное окружение через user namespaces: корень = squashfs (или overlay), пробрасывает $HOME, /dev, /tmp, X11/Wayland сокеты, накладывает /etc/resolv.conf и другие файлы хоста.

## Планы
- [x] Автономное обновление капсул
- [x] Поддержка nvidia (тестируется)
- [x] Сборка капсул без root-прав
- [ ] Сборка утилит и бинарных встроенных зависимостей капсулы под ALT Linux своими силами на ALS
- [ ] Добавить управление разрешениями для капсул

# Credit
- [Conty](https://github.com/Kron4ek/Conty) - For the idea
- [Epm](https://github.com/Etersoft/eepm) - For packages
- [Stplr](https://altlinux.space/stapler/stplr) - For packages
