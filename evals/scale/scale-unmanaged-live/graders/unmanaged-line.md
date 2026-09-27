---
type: regex
pattern: '^UNMANAGED:(?=[^\n]*\bteam\-02\/auth\b)(?=[^\n]*\bteam\-03\/auth\b)(?=[^\n]*\bteam\-05\/cron\b)(?=[^\n]*\bteam\-05\/auth\b)(?=[^\n]*\bteam\-05\/notify\b)(?=[^\n]*\bteam\-11\/api\b)(?=[^\n]*\bteam\-11\/notify\b)(?=[^\n]*\bteam\-12\/web\b)(?=[^\n]*\bteam\-18\/web\b)(?=[^\n]*\bteam\-19\/cron\b)(?=[^\n]*\bteam\-25\/search\b)(?=[^\n]*\bteam\-30\/cache\b)[^,\n]+(?:,[^,\n]+){11}[ \t]*$'
flags: im
target: last_message
---
