# Differences from Traditional FChannel

Fedichan's primary objective is to improve the quality of life for those hosting
FChannel instances, so a number of changes were made to ensure that goal.
These are non-compatible but hopefully have good rationale behind them.

- Some artifacts from the client/server days have been merged
  - Long ago, FChannel-Server and FChannel-Client were two separate projects,
    but they were merged together.
    However as a result, some codepaths use HTTP requests to itself, which
    wastes time and in some scenarios can fail.
  - Not all have been removed, but all are hereby declared **deprecated**.
    They may stop working at any time in the future without any notice.
- Mod authentication has been replaced with a traditional username/password
  system
  - Before the change, authentication was done using identifier/code, which is
    like a username and password, however the code was stored unhashed and was
    used for numerous other functions such as the board captcha.
  - Unfortunately, existing login information has been removed.
    The admin credentials will be printed upon the first start of Fedichan but
    because the password is now hashed, it will no longer be printed afterwards.
