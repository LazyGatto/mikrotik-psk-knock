# RouterOS

Здесь будут храниться RouterOS scripts, firewall snippets и export-фрагменты для ROS-only прототипа.

## Прототипы

- [prototype-stage-token.rsc](prototype-stage-token.rsc) - минимальный end-to-end прототип
  `stage1 -> stage2 -> token-hit -> scheduler -> allowed` со статическим тестовым token payload.

Первый прототип намеренно не реализует PSK/time-token генерацию. Его задача - проверить bridge между
firewall packet path и scheduler/script runtime.
