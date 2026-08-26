# План: онбординг сервисного пользователя на роутере — issue #33

Дата: 2026-08-27. Цель: провижн ходит на роутер под собственным пользователем
`mkpk` с ключом инсталляции и минимальными правами, а не под `admin`.

## Архитектурная проверка

1. **Документы:** `docs/deploy-docker.md` (раздел про ключ), `docs/admin-app.md`,
   `docs/threat-model.md` (сервисный юзер сужает поверхность — это стоит
   зафиксировать), ман-страница.
2. **Инварианты:** протокол, токен, инвайты не затрагиваются. Затрагивается
   **состояние роутера за пределами нашего слоя** (`/user`, `/user group`) —
   значит только по явному действию, с обратной операцией и внятным логом.
3. **Совместимость:** существующие роутеры продолжают работать под текущим
   юзером; онбординг — опциональное действие.
4. **Новые файлы:** `client/internal/deploy/serviceuser.go` (+тесты),
   ручка `/api/router/serviceuser`, кусок UI в карточке роутера.
5. **Зависимости:** новых нет — переиспользуем `deploy.Client` (`upload`, `Run`).
6. **Живой CHR:** обязателен **до** реализации — см. HALT ниже.

## HALT H-2 (открыт)

Минимальный набор политик группы неизвестен и проверяется только вживую.
Гипотеза: `ssh,ftp,read,write,policy,test` (+ `sensitive`?). Реализация
начинается после подтверждения набора на CHR.

## Проверка на CHR (делает maintainer)

Публичную часть ключа берём из своей же инсталляции провижна:

```sh
# на хосте с провижном
sudo docker compose -f /opt/mkpk/compose.yaml exec provision \
  mkpk-provision sshkey show --config /data/mkpk.yaml
```

На CHR (под админом), подставив свою строку ключа:

```routeros
/user group add name=mkpk policy=ssh,ftp,read,write,policy,test \
  comment="mkpk provisioning service account"
/user add name=mkpk group=mkpk comment="mkpk-provision service account"

# ключ: залить mkpk-provision.pub в Files, затем
/user ssh-keys import user=mkpk public-key-file=mkpk-provision.pub
```

Затем в карточке роутера в провижне поставить `user=mkpk`, путь к ключу —
`/data/ssh/id_ed25519`, и прогнать по порядку:

1. **Deploy → Status** — должен увидеть роутер и посчитать хеши
   (проверяет `ssh`, `read`);
2. **Deploy** — должен пройти целиком
   (проверяет `ftp` для scp, `write` и `policy` для скриптов/шедулеров);
3. постучаться клиентом — доступ открывается (слой реально рабочий);
4. **Deploy → Status** ещё раз — «up to date», без дрейфа;
5. **Uninstall** — снимается чисто.

Что фиксируем по результату:

- прошло всё → набор `ssh,ftp,read,write,policy,test` подтверждён;
- упало на scp → не хватает `ftp`;
- упало на `/import` со скриптами → не хватает `policy` (или `test`);
- Status показывает `NOT_INSTALLED` при установленном слое → не хватает прав на
  чтение исходника скрипта, пробуем добавить `sensitive`.

Лог падения нужен целиком: он и есть ответ, какой политики не хватило.

## Шаги реализации (после подтверждения)

| # | Что | Зона |
| --- | ----- | ------ |
| 1 | `deploy.EnsureServiceUser(pub, name)` — группа, юзер, импорт ключа, идемпотентно | `internal/deploy/serviceuser.go` |
| 2 | `deploy.RemoveServiceUser(name)` — обратное действие | там же |
| 3 | API `/api/router/serviceuser` (POST create / DELETE) с одноразовыми бутстрап-кредами | `internal/web` |
| 4 | UI: кнопка в блоке SSH карточки роутера + подтверждение + лог | `internal/web/assets` |
| 5 | CLI `mkpk-provision serviceuser add|remove --router …` | `cmd/mkpk-provision` |
| 6 | Доки и threat-model | docs |

Готово, когда: `bash scripts/verify.sh` = 0; на живом CHR онбординг проходит с
нуля, деплой работает под сервисным юзером, снятие чистое.
