# TNPG: Push Gateway

This is a push notifications adapter which communicates with Sunrise Push Gateway (TNPG).

TNPG is a proprietary service intended to simplify deployment of on-premise installations.
Deploying a Sunrise server without TNPG requires [configuring Google FCM](../fcm/) with your own credentials, recompiling Android and iOS clients, releasing them to PlayStore and AppStore under your own accounts. It's usually time consuming and relatively complex.

TNPG solves this problem by letting Sunrise LLC (the company behind Sunrise) to send push notifications on your behalf: you hand a notification over to TNPG, TNPG sends it to the clients using its own credentials and certificates. Internally it uses [Google FCM](https://firebase.google.com/docs/cloud-messaging/) and as such supports the same platforms as FCM. The main advantage of using TNPG over FCM is simplicity of configuration: you can use stock mobile clients with your custom Sunrise server, all is needed is a configuration update on the server.

## Configuring TNPG adapter

### Obtain TNPG token

1. Register at https://console.sunrise.co and create an organization.
2. Get the TPNG token from the _Self hosting_ &rarr; _Push Gateway_ section by following the instructions there.

### Configure the server
Update the server config [`sunrise.conf`](../../sunrise.conf#L413), section `"push"` -> `"name": "tnpg"`:
```js
{
  "enabled": true,
  "org": "myorg", // Short name (URL) of the organization you registered at console.sunrise.co
  "token": "SoMe_LonG.RaNDoM-StRiNg.12345" // authentication token obtained from console.sunrise.co
}
```
Make sure the `fcm` section is disabled `"enabled": false` or removed altogether.
