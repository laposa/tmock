# T-Mock

TO-DO


## Client Access Tokens

Client Access Tokens could used for automated tests and belong to a single Client.

The only allowed API call is:

```http
PATCH /api/client/:clientId
```

Body fields allowed:
- `enabled` (`true` / `false`)
- `scenarios` (array)

The token must be specified as the `x-client-token` HTTP header.

On the Clients screen, the new **Tokens** column works like this:
- If a token exists, show **View**
- If no token exists, show **(+)** to generate one

The token modal shows the token in a read-only field, lets you copy it, revoke it, or close the modal.
