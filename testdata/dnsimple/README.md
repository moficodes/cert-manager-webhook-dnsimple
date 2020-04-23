# Solver testdata directory

1. Copy the two *.example file and remove the .example extension
2. Find the accountID for your dnsimple account. Its in the same place you will go for [API Access Tokens](https://support.dnsimple.com/articles/api-access-token/)
3. Find the [API Access Token](https://support.dnsimple.com/articles/api-access-token/) for your dnsimple account.
4. Replace your accountId in config.json file
5. Base64 encode your API Access Token
  ```bash
  echo -n "<api-access-token>" | base64
  ```
6. Replace that in the accessToken field in the `dnsimple-credentials.yaml` file.

