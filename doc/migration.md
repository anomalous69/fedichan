# Migrating from FChannel

Fedichan is meant to continue development but had to make some changes along the
way to improve quality of life.
As such, it is mostly easy to migrate over however it is not without
intervention.

## Caveats

- **Accounts**: They're all gone, you gotta set them up again.
  I completely redid authentication, you're now using a username and password,
  whatever was there is not migrated.
  Admin account's password will be printed to the terminal upon first run.
- **Stale captchas**: I didn't delete them.
  Open up `psql`, grab a list of filenames to remove from `public/` before you
  migrate with `SELECT identifier FROM verification WHERE type = 'captcha';`.

## Steps

This list is short now but will likely grow as time goes on.
Care will be taken to make this as automatic as possible.

**FOR THE LOVE OF ALL THAT IS HOLY, TAKE BACKUPS.**
I do not guarantee this will work for you, only that it worked for me.
Fedichan makes **destructive changes to the database**.
The data it deletes (asides from some tables) is mostly useless and the fields
even completely unused, but keep this in mind.

1. Ensure you are running the latest version of FChannel before migrating
2. Install Fedichan like how you would FChannel
3. Move `config/config-init` to `fchan.cfg`.
4. You're done, hopefully. Run `./fchan`.

Take note of the admin password that is printed to the terminal on the first
run, as you won't get a chance to see it again unless you delete the admin user
from the Postgres console.
