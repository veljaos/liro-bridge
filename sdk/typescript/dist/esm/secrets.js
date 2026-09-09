/**
 * Where the device secret lives, and what stops it going anywhere else.
 *
 * The device secret is the whole of an application's authority to ask
 * for a signature. `PROTOCOL.md` §2.4 is blunt about where it belongs —
 * on your server, and nowhere a browser can reach — and this SDK does
 * not choose for you: {@link ConnectOptions.secretStore} is required and
 * has no default. Not a file path with a sensible fallback, not
 * `~/.liro`. Somebody has to decide where a signing secret is kept, and
 * it is not this library.
 */
import { chmod, mkdir, readFile, rename, rm, stat, writeFile } from 'node:fs/promises';
import { dirname, join } from 'node:path';
import { randomBytes } from 'node:crypto';
import { execFile } from 'node:child_process';
import { userInfo } from 'node:os';
import { LiroError } from './errors.js';
/**
 * The marker {@link StoredPairing.toJSON} puts where the secret would
 * be.
 *
 * A `SecretStore` that persists `JSON.stringify(pairing)` — which is
 * the first thing anybody writes — stores this string instead of a
 * secret. {@link StoredPairing.deserialise} recognises it and throws a
 * sentence naming `serialise()`, so the mistake surfaces the first time
 * the store is read back rather than as `AUTH_FAILED` on some later
 * request, which is the least diagnosable answer this protocol has.
 */
export const REDACTED_SECRET = '[redacted: persist pairing.serialise(), not JSON.stringify(pairing)]';
const inspectCustom = Symbol.for('nodejs.util.inspect.custom');
/**
 * Where the device secret actually lives.
 *
 * A `WeakMap` outside the class rather than a field on it, because a
 * field is a property and a property is something that can be reached:
 * spread it, enumerate it, `util.inspect` it with `showHidden`, or —
 * for a `#private` field — read it out of a heap snapshot the same way.
 * Nothing here can be reached from an instance at all. It is also the
 * only version of this that cannot be broken by a future change to how
 * Node prints objects, which is the failure this is guarding against.
 */
const secretBytes = new WeakMap();
/**
 * One paired application: what the agent knows it as, and the secret it
 * authenticates with.
 *
 * The secret is deliberately awkward to leak and deliberately easy to
 * store on purpose:
 *
 * - It is a **non-enumerable** property, so `{...pairing}`,
 *   `Object.keys(pairing)` and a `for...in` do not see it.
 * - `JSON.stringify(pairing)` produces {@link REDACTED_SECRET} in its
 *   place.
 * - `console.log(pairing)` prints the redacted form, because Node's
 *   inspector is given one.
 * - {@link serialise} is the one way to get it out, and it is named so
 *   that using it is a decision.
 */
export class StoredPairing {
    /** The identifier the agent issued at pairing; the `X-Liro-App-Id` header. */
    appId;
    /** The display name bound at pairing — what the person sees above every signature. */
    applicationName;
    /** The origin bound at pairing, shown verbatim on the agent's window. */
    origin;
    constructor(appId, applicationName, origin, deviceSecret) {
        this.appId = appId;
        this.applicationName = applicationName;
        this.origin = origin;
        secretBytes.set(this, Uint8Array.from(deviceSecret));
    }
    /**
     * The device secret's raw bytes, for signing a request.
     *
     * A copy, so that a caller holding the result cannot reach back into
     * the pairing and change what it authenticates with.
     */
    deviceSecretBytes() {
        const bytes = secretBytes.get(this);
        if (bytes === undefined) {
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: 'this pairing carries no device secret; it was not built by StoredPairing’s own constructor',
            });
        }
        return Uint8Array.from(bytes);
    }
    /**
     * The plain object to persist, secret included.
     *
     * This is the only place the secret leaves this class, and it is
     * named so that a `SecretStore` implementation has to mean it. Pair it
     * with {@link StoredPairing.deserialise} on the way back.
     */
    serialise() {
        return {
            appId: this.appId,
            applicationName: this.applicationName,
            origin: this.origin,
            deviceSecret: Buffer.from(this.deviceSecretBytes()).toString('base64'),
        };
    }
    /** Rebuilds a pairing from {@link serialise}'s output. */
    static deserialise(record) {
        if (typeof record !== 'object' || record === null) {
            throw new LiroError('SECRET_STORE_INVALID', { detail: 'the stored pairing is not an object' });
        }
        const r = record;
        const appId = r['appId'];
        const applicationName = r['applicationName'];
        const origin = r['origin'];
        const deviceSecret = r['deviceSecret'];
        if (deviceSecret === REDACTED_SECRET) {
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: 'the stored pairing has a redaction marker where its device secret should be. A SecretStore must ' +
                    'persist pairing.serialise() and rebuild with StoredPairing.deserialise(); JSON.stringify(pairing) ' +
                    'deliberately leaves the secret out so it cannot reach a log by accident',
            });
        }
        for (const [name, value] of [
            ['appId', appId],
            ['applicationName', applicationName],
            ['origin', origin],
            ['deviceSecret', deviceSecret],
        ]) {
            if (typeof value !== 'string' || value === '') {
                throw new LiroError('SECRET_STORE_INVALID', {
                    detail: `the stored pairing has no usable "${name}"`,
                });
            }
        }
        const bytes = Buffer.from(deviceSecret, 'base64');
        if (bytes.length !== 32) {
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: `the stored device secret decodes to ${bytes.length} bytes, and a device secret is 32`,
            });
        }
        return new StoredPairing(appId, applicationName, origin, new Uint8Array(bytes));
    }
    /** What `JSON.stringify` sees: everything but the secret. */
    toJSON() {
        return {
            appId: this.appId,
            applicationName: this.applicationName,
            origin: this.origin,
            deviceSecret: REDACTED_SECRET,
        };
    }
    /** What `console.log` and `util.inspect` see. */
    [inspectCustom]() {
        return `StoredPairing { appId: '${this.appId}', applicationName: ${JSON.stringify(this.applicationName)}, origin: ${JSON.stringify(this.origin)}, deviceSecret: <32 bytes, withheld> }`;
    }
    /** Never the secret, in any interpolation anywhere. */
    toString() {
        return this[inspectCustom]();
    }
}
const isWindows = process.platform === 'win32';
/**
 * A {@link SecretStore} that keeps pairings in one JSON file at a path
 * the caller names.
 *
 * There is no default path on purpose. A device secret's location is a
 * decision, and a library that picks one for you has made it on your
 * behalf in every deployment that never thought about it.
 *
 * Permissions are set as tightly as the platform allows, and a failure
 * to set them is an error rather than a warning: a signing secret in a
 * world-readable file is the thing this class exists to avoid.
 *
 * - **POSIX** — the file is `0600`, owner read/write and nothing else.
 * - **Windows** — the file's inherited ACL is replaced with a single
 *   entry granting the current user full control, via `icacls`. That is
 *   the equivalent of `0600` on a filesystem that has no mode bits.
 *
 * For a server application, prefer your platform's secrets manager and
 * implement {@link SecretStore} against it.
 */
export class FileSecretStore {
    path;
    /** @param path the file to keep pairings in. Its directory is created if it does not exist. */
    constructor(path) {
        if (typeof path !== 'string' || path.trim() === '') {
            throw new LiroError('CONFIGURATION_INVALID', {
                detail: 'FileSecretStore needs a path; there is deliberately no default',
            });
        }
        this.path = path;
    }
    async get(appName) {
        const file = await this.read();
        const record = file.pairings[appName];
        if (record === undefined) {
            return null;
        }
        return StoredPairing.deserialise(record);
    }
    async set(appName, pairing) {
        const file = await this.read();
        file.pairings[appName] = pairing.serialise();
        await this.write(file);
    }
    async clear(appName) {
        const file = await this.read();
        if (!(appName in file.pairings)) {
            return;
        }
        delete file.pairings[appName];
        await this.write(file);
    }
    async read() {
        let text;
        try {
            text = await readFile(this.path, 'utf8');
        }
        catch (err) {
            if (err.code === 'ENOENT') {
                return { version: 1, pairings: {} };
            }
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: `the secret store at ${this.path} could not be read`,
                cause: err,
            });
        }
        let parsed;
        try {
            // A byte-order mark is what every ordinary way of editing a file
            // on Windows leaves behind, and JSON.parse refuses one at offset
            // 1. The agent's own configuration loader strips the same three
            // bytes for the same reason (decision D-157).
            parsed = JSON.parse(text.replace(/^﻿/, ''));
        }
        catch (err) {
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: `the secret store at ${this.path} is not valid JSON`,
                cause: err,
            });
        }
        const file = parsed;
        if (typeof file !== 'object' || file === null || typeof file.pairings !== 'object' || file.pairings === null) {
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: `the secret store at ${this.path} does not hold a pairing map`,
            });
        }
        return { version: 1, pairings: { ...file.pairings } };
    }
    async write(file) {
        const directory = dirname(this.path);
        await mkdir(directory, { recursive: true });
        // Written beside its destination and renamed over it, so a reader
        // never sees a half-written store and a failure part-way leaves the
        // previous one intact. The temporary file is in the same directory
        // so the rename is within one volume and therefore atomic — the
        // same reasoning the agent applies to a signed document (D-165).
        const temporary = join(directory, `.liro-secrets-${randomBytes(8).toString('hex')}.tmp`);
        const body = `${JSON.stringify(file, null, 2)}\n`;
        try {
            await writeFile(temporary, body, { encoding: 'utf8', mode: 0o600 });
            await restrictToCurrentUser(temporary);
            await rename(temporary, this.path);
        }
        catch (err) {
            await rm(temporary, { force: true }).catch(() => undefined);
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: `the secret store at ${this.path} could not be written`,
                cause: err,
            });
        }
        // Set again after the rename: on Windows the destination may carry
        // its own inherited ACL, and on POSIX an unusual umask can defeat
        // the mode passed to writeFile.
        await restrictToCurrentUser(this.path);
    }
}
/**
 * Restricts `path` to the current user, and throws if it cannot.
 *
 * A secret whose permissions could not be set is not stored quietly.
 * The alternative — warn and carry on — is how a signing secret ends up
 * readable by every account on a terminal server, which SPEC §14.1 says
 * is the ordinary configuration for this product's users rather than an
 * edge case.
 */
async function restrictToCurrentUser(path) {
    if (!isWindows) {
        await chmod(path, 0o600);
        const mode = (await stat(path)).mode & 0o777;
        if (mode !== 0o600) {
            throw new LiroError('SECRET_STORE_INVALID', {
                detail: `could not restrict ${path} to its owner: it is mode 0${mode.toString(8)}`,
            });
        }
        return;
    }
    // Windows has no mode bits, so `chmod` above would be a no-op that
    // looked like it worked. `icacls` is the platform's own tool for this
    // and ships with every supported Windows.
    //
    //   /inheritance:r      break inheritance from the directory
    //   /grant:r <user>:(F) replace this user's own entry with full control
    //   /remove:g <SIDs>    take the access away from every principal that
    //                       would let *another ordinary account on this
    //                       machine* read the file
    //
    // The four SIDs are Everyone, Authenticated Users, Users and
    // Interactive. They are given as SIDs and not as names on purpose:
    // "BUILTIN\Administrators" is "VGRAĐENO\Administratori" on a Serbian
    // Windows, and this product's users are on Serbian Windows. A SID is
    // the same on every installation in every language.
    //
    // That is the guarantee this claims, and it holds from a zero exit
    // code without parsing anything: inheritance is broken, this user has
    // full control, and no group that another ordinary account belongs to
    // has any access. Measured on Windows 11 26200, the whole call in fact
    // leaves exactly one entry — the user — but that is icacls' own
    // behaviour when /remove is present and it is not what is relied on
    // here. SYSTEM and the Administrators group surviving would be the
    // ordinary Windows arrangement for a per-user credential file and is
    // not a weakness worth chasing: an administrator can take ownership of
    // any file on the machine regardless of what its DACL says.
    const user = windowsAccountName();
    await new Promise((resolve, reject) => {
        execFile('icacls', [
            path,
            '/inheritance:r',
            '/grant:r',
            `${user}:(F)`,
            '/remove:g',
            '*S-1-1-0',
            '*S-1-5-11',
            '*S-1-5-32-545',
            '*S-1-5-4',
        ], { windowsHide: true }, (err, _stdout, stderr) => {
            if (err) {
                reject(new LiroError('SECRET_STORE_INVALID', {
                    detail: `could not restrict ${path} to ${user} with icacls (${String(stderr).trim() || err.message}). ` +
                        'A device secret is not written with permissions this SDK could not set — put the store ' +
                        'somewhere writable, or implement SecretStore against your own secrets manager',
                    cause: err,
                }));
                return;
            }
            resolve();
        });
    });
}
/** `DOMAIN\user`, or the bare user name when there is no domain. */
function windowsAccountName() {
    const domain = process.env['USERDOMAIN'];
    const name = process.env['USERNAME'] ?? userInfo().username;
    return domain ? `${domain}\\${name}` : name;
}
