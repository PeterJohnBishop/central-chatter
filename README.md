# central-chatter

Generate a public key: ssh-keygen -t ed25519

Connect to the server: ssh <YourUsername>@<ServerIP> -p 23234

- If new user, app authentication is false sending you to request system access
    - Once approved, the TUI will automatically update, sending you to the main app
- If your public key is registgered to your user name in the database, app authentication is true sending you to the main app
- If promoted to 'admin' role the user table to approve, revoke, promot or demote appears above the main app
