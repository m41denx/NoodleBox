## 🍜📦

```
 [REQUEST] → RateLimit → NoodleAuth → AnswerCollector → Cache → [ORIGIN]
[RESPONSE] ←  ← Injector ← Patcher ← NoodleAuth(Refresh) ← [ORIGIN]
```


AppToken Target:
`https://edu.vsu.ru/login/token.php`
POST: `username=AAA&password=XXX&service=moodle_mobile_app`
INVALID: JSON["error"]
VALID: JSON["token"]