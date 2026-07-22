# RouterOS

Здесь будут храниться RouterOS scripts, firewall snippets и export-фрагменты для ROS-only прототипа.

## Прототипы

- [prototype-stage-token.rsc](prototype-stage-token.rsc) - минимальный end-to-end прототип
  `stage1 -> stage2 -> token-hit -> scheduler -> allowed` со статическим тестовым token payload.
- [prototype-time-token.rsc](prototype-time-token.rsc) - прототип с PSK-derived time-token,
  RouterOS-side `sha512` и scheduler-обновлением `content` для текущего/предыдущего bucket.

Первый прототип намеренно не реализует PSK/time-token генерацию. Его задача - проверить bridge между
firewall packet path и scheduler/script runtime.

`prototype-time-token.rsc` хранит demo profile values в отдельном persistent RouterOS script
`mkpk-tt-profile-demo`. PSK все еще находится в script source, поэтому production-вариант должен отдельно
ограничить права на чтение scripts и описать ротацию секретов.
