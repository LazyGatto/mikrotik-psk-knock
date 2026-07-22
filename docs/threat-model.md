# Модель угроз

## Что хотим улучшить

Главная цель - сделать лучше, чем:

```text
public dst-nat -> service exposed 24/7
```

И лучше, чем обычный port knocking, где секретом является только последовательность портов.

## Trust zones

Provisioning выполняется из safe/admin сети, где допустим полный management-доступ к MikroTik:
импорт `.rsc`, проверка firewall/NAT/scripts/schedulers, reboot-тесты и ротация секретов.

Runtime-доступ roaming клиента выполняется из unsafe сети. В этой фазе клиент не должен иметь SSH/API к
RouterOS и не должен полагаться на обратный management channel. Он отправляет только staged UDP knock и
PSK-derived time-token; RouterOS открывает только observed source IP на короткий timeout.

Break-glass/admin mode через SSH/API допустим как отдельный осознанный режим из safe среды, но не является
частью основной security-модели stealth UDP-token flow.

## Защищает от

- массового сканирования Интернета;
- случайного обнаружения внутреннего сервиса;
- простого перебора knock-портов;
- открытия доступа без знания PSK;
- повторного использования старых токенов после истечения time bucket;
- повторного использования уже принятого token/bucket после scheduler пометки `used`;
- незаметного открытия доступа, если включены уведомления.

## Не защищает полностью от

- replay валидного токена до момента, когда scheduler пометил token/bucket как `used`;
- DoS через принудительную collision: on-path атакующий может повторить валидный token с другого IP в
  тот же polling interval, заставив RouterOS fail-closed сжечь bucket без открытия доступа;
- активного MITM в момент knock;
- компрометации клиента и утечки PSK;
- компрометации MikroTik и чтения scripts/secrets;
- DoS по UDP/firewall matcher;
- ошибок конфигурации NAT/firewall.

## Replay caveat

PSK-derived token или HMAC защищает от подделки, но не запрещает повтор уже увиденного валидного пакета.

Для полной replay-защиты нужен nonce cache:

```text
nonce accepted once -> stored as used -> repeated nonce rejected
```

В ROS-only firewall/content дизайне такой verifier, вероятно, недоступен. Поэтому replay ограничивается коротким time bucket.

Дополнительный ROS-only компромисс: token-hit address-list плюс scheduler polling.

```text
firewall accepts valid token -> token-hit list
scheduler every ~1s -> allow one src -> mark token/bucket used
```

Это сужает replay window примерно до `scheduler interval + processing time`, если used-marker живет
дольше полного окна приема `now+prev`. При interval 1 секунда это лучше, чем replay window длиной во весь
time bucket, но все еще не является строгой атомарной replay protection.

Если за один polling interval пришли несколько hits с одним token, scheduler не должен разрешать все адреса. Консервативная политика: открыть только первый hit при надежном порядке записей или сжечь token без открытия доступа и отправить alert.

## Reboot и runtime state

После reboot RouterOS dynamic state нужно считать потерянным:

- временно разрешенные `allowed` entries исчезают или должны быть очищены;
- `stage`/`token-hit` entries исчезают;
- in-memory used-state не считается надежным.

Безопасный failure mode: после reboot доступ закрыт, пока scheduler не пересчитает актуальные token
rules. Клиент должен повторить knock. Нельзя проектировать режим, где старый runtime token или
неинициализированное состояние после reboot открывает доступ шире, чем до reboot.

Нужно учитывать практическую особенность RouterOS: scheduler-обновление firewall rule `content` сохраняет
это значение как config state. После reboot до первого startup tick scheduler-а rules могут кратко
содержать stale token. Это не должно считаться полноценной fail-closed гарантией, пока не измерено на CHR
и не добавлен более строгий startup guard при необходимости.

Базовый reboot-тест CHR подтвердил хороший основной failure mode: dynamic `allowed` state после reboot
не сохранился, persistent scripts/rules/scheduler поднялись, scheduler пересчитал token rules, а клиенту
потребовалось выполнить knock заново. Остаточный риск - только короткое startup окно stale persisted token
content до первого успешного пересчета.

Проверенный startup guard снижает этот риск: startup script сначала отключает token rules и ставит
invalid content, затем запускает poller для актуального пересчета. Полностью доказать отсутствие окна до
самого первого startup script tick пока нельзя, поэтому это остается residual risk ROS-only режима.

## Сравнение уровней

```text
Просто dst-nat наружу
  сервис постоянно виден и атакуем

Обычный port knocking
  сервис скрыт, но последовательность можно подсмотреть и повторить

Staged UDP + PSK/SHA512 time-token на RouterOS
  последовательность недостаточна, нужен актуальный токен
  replay ограничен time bucket или примерно 1s при polling single-use bucket

SSH/API/external verifier with nonce cache
  более строгая криптография и полноценная replay protection
  но требует management channel и не является stealth UDP-token режимом
```

## Практический security target для ROS-only

ROS-only режим нужно описывать как:

```text
PSK time-token gated port opening for RouterOS
```

А не как полноценный HMAC port knocking.

Целевое честное описание:

- защищает от сканеров, шума и неавторизованного открытия без PSK;
- существенно лучше обычного port knocking;
- значительно безопаснее постоянного публичного `dst-nat`;
- имеет ограниченное replay window;
- может сужать replay window через polling-based single-use bucket;
- не требует SSH/API на runtime path;
- не заменяет VPN или полноценный криптографический verifier в высокорисковых сценариях.

Если RouterOS SSH/API доступен клиенту в момент открытия, можно напрямую добавить source IP в address-list.
Это полезно как отдельный admin-mode, но противоречит основной цели UDP-token режима: держать production host
максимально невидимым и не выставлять management path как условие работы knock.

## Static IP не является целевым сценарием

Привязка token к source IP могла бы уменьшить replay с другого адреса, но основной сценарий проекта - dynamic/roaming клиенты.

Если source IP заранее известен, authenticated knock обычно не нужен как основной механизм. Такой адрес проще заранее добавить в static allow-list или обслуживать отдельной firewall policy.

Поэтому в основной модели угроз token не привязан к source IP, а replay mitigation строится через staged knock, короткие buckets, token-hit polling и used-state.
