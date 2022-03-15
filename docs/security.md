# Security

## Authentication

The authentication flow is as follows:

### Registration

1. Client makes a request for registration by providing a random string which uniquely identifies the client.
2. Server generates a 32 characters (base64) long auth key (192 bits of total entropy). 
3. Server calculates a hash (bcrypt) for the key and saves it to the embedded persistent k/v store, mapping to the provided client id.
4. The auth key is returned to the client.

### Client Authentication

1. Client makes a request for authentication by providing its client id and associated authentication key.
2. Server fetches the hashed key from the embedded persistent k/v store.
3. Server calculates the hash for the provided auth key and compares it to the stored value.
4. Authentication is considered successful if the compare operation returns no error.
