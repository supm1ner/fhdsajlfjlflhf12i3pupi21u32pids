import * as TinodeModule from 'tinode-sdk';

const API_KEY = 'AQEAAAABAAD_rAp4DJh05a1HAwFT3A6K';
const HOST = 'localhost:6060';
const APP_NAME = 'SunriseWeb';

let client = null;

export function getClient() {
  if (!client) {
    const Tinode = TinodeModule.Tinode ?? TinodeModule.default?.Tinode;
    client = new Tinode({ appName: APP_NAME, host: HOST, apiKey: API_KEY, transport: 'ws', secure: false });
  }
  return client;
}

export function connect() {
  return new Promise((resolve, reject) => {
    const c = getClient();
    const to = setTimeout(() => reject(new Error('Connection timeout')), 10000);
    c.onConnect = () => { clearTimeout(to); resolve(c); };
    c.onDisconnect = (err) => { clearTimeout(to); reject(err); };
    c.connect().catch(reject);
  });
}

export function login(login, password) {
  return getClient().login('basic', `${login}:${password}`, true);
}

export function createAccount(login, password, name, email) {
  const c = getClient();
  const secret = btoa(`${login}:${password}`);
  const desc = { public: { fn: name } };
  const cred = email ? [{ meth: 'email', val: email }] : undefined;
  return c.createAccount('basic', secret, true, desc, undefined, cred);
}

export function subscribe(topicName) {
  return getClient().subscribe(topicName);
}

export function getTopics() {
  return getClient().getTopics();
}

export function publish(topic, content) {
  return getClient().publish(topic, { txt: content });
}

export function getMessages(topic) {
  return getClient().getMessages(topic);
}

export function logout() {
  if (client) {
    client.disconnect();
    client = null;
  }
}
