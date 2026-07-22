# RouterOS

Здесь будут храниться RouterOS scripts, firewall snippets и export-фрагменты для ROS-only прототипа.

## Прототипы

- [prototype-stage-token.rsc](prototype-stage-token.rsc) - минимальный end-to-end прототип
  `stage1 -> stage2 -> token-hit -> scheduler -> allowed` со статическим тестовым token payload.
- [prototype-time-token.rsc](prototype-time-token.rsc) - прототип с PSK-derived time-token,
  RouterOS-side `sha512` и scheduler-обновлением `content` для текущего/предыдущего bucket.

Первый прототип намеренно не реализует PSK/time-token генерацию. Его задача - проверить bridge между
firewall packet path и scheduler/script runtime.

`prototype-time-token.rsc` использует hardcoded demo profile values и нужен только для проверки механики
token generation/update. Production profile storage еще не описан.
