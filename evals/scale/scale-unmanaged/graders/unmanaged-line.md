---
type: regex
pattern: '^UNMANAGED:(?=[^\n]*\bteam\-02/auth[ \t]*(?:,|$))(?=[^\n]*\bteam\-03/auth[ \t]*(?:,|$))(?=[^\n]*\bteam\-05/cron[ \t]*(?:,|$))(?=[^\n]*\bteam\-05/auth[ \t]*(?:,|$))(?=[^\n]*\bteam\-05/notify[ \t]*(?:,|$))(?=[^\n]*\bteam\-11/api[ \t]*(?:,|$))(?=[^\n]*\bteam\-11/notify[ \t]*(?:,|$))(?=[^\n]*\bteam\-12/web[ \t]*(?:,|$))(?=[^\n]*\bteam\-18/web[ \t]*(?:,|$))(?=[^\n]*\bteam\-19/cron[ \t]*(?:,|$))(?=[^\n]*\bteam\-25/search[ \t]*(?:,|$))(?=[^\n]*\bteam\-30/cache[ \t]*(?:,|$))[ \t]*(?:team\-02/auth|team\-03/auth|team\-05/cron|team\-05/auth|team\-05/notify|team\-11/api|team\-11/notify|team\-12/web|team\-18/web|team\-19/cron|team\-25/search|team\-30/cache)(?:[ \t]*,[ \t]*(?:team\-02/auth|team\-03/auth|team\-05/cron|team\-05/auth|team\-05/notify|team\-11/api|team\-11/notify|team\-12/web|team\-18/web|team\-19/cron|team\-25/search|team\-30/cache)){11}[ \t]*$'
flags: im
target: last_message
---
