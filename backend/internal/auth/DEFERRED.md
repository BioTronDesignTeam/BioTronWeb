# C2 — Session is never re-validated for org membership

`RequireSession` only checks that a session row exists and is unexpired. Org
membership is checked **once, at GitHub login**. With a week-long TTL, someone
removed from `BioTronDesignTeam` keeps access until the cookie dies. There is
no periodic re-check and no “kill this operator’s sessions” hook beyond ban
(which only works if staff notice).

**Deferred.** Tools are read-only today. Revisit when a **control / e-stop**
path can move hardware.

**Then:** re-check `IsOrgMember` on sensitive actions (and/or every few
minutes), shorten `SESSION_TTL`, and/or add staff “revoke sessions” for an
operator. Ban already deletes sessions — off-boarding should use that until
this lands.
