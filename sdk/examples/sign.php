<?php
/**
 * Sign a PDF with Liro Bridge, from PHP, with nothing but what ships in
 * the standard build.
 *
 *   php sign.php ugovor.pdf
 *
 * Reads the pairing from LIRO_APP_ID and LIRO_SECRET when they are set,
 * and pairs otherwise — the agent shows six digits on the person's
 * screen and this asks for them.
 *
 * Written against docs/PROTOCOL.md. The four mistakes a first
 * integration makes are avoided here by construction:
 *
 *   1. a wrong field name          — every name comes from §5.2's example
 *   2. origin missing from confirm — ONE origin variable, used in both calls
 *   3. a body altered between being hashed and being sent — the bytes are
 *      built once and both hashed and sent
 *   4. a Content-Type on a request with no body — only set when there is one
 */

declare(strict_types=1);

// --- README ---
$discovery = getenv('LOCALAPPDATA') . DIRECTORY_SEPARATOR . 'Liro' . DIRECTORY_SEPARATOR . 'bridge.json';
if (!is_file($discovery)) {
    fwrite(STDERR, "Liro Bridge is not running: no $discovery\n");   // never scan ports
    exit(1);
}
$base = 'http://127.0.0.1:' . json_decode(file_get_contents($discovery), true)['port'];

/** One request, signed when a secret is given. Returns [status, decoded body]. */
function liro_call(string $method, string $path, ?array $body = null,
                   ?string $appId = null, ?string $secret = null): array {
    global $base;
    $raw = $body === null ? '' : json_encode($body, JSON_UNESCAPED_SLASHES);  // hashed AND sent
    $headers = [];
    if ($raw !== '') {
        $headers[] = 'Content-Type: application/json';        // only when there is a body
    }
    if ($secret !== null) {
        $ts    = (string) time();
        $nonce = bin2hex(random_bytes(16));                   // never mt_rand()
        $canon = implode("\n", [strtoupper($method), $path, $ts, $nonce, hash('sha256', $raw)]);
        $sig   = hash_hmac('sha256', $canon, base64_decode($secret));
        $headers[] = "X-Liro-App-Id: $appId";
        $headers[] = "X-Liro-Timestamp: $ts";
        $headers[] = "X-Liro-Nonce: $nonce";
        $headers[] = "X-Liro-Signature: $sig";
    }
    $context = stream_context_create(['http' => [
        'method'        => strtoupper($method),
        'header'        => implode("\r\n", $headers),
        'content'       => $raw,
        'ignore_errors' => true,        // read 4xx and 5xx bodies rather than throwing
        'timeout'       => 30,
    ]]);
    $text   = file_get_contents($base . $path, false, $context);
    $status = (int) explode(' ', $http_response_header[0])[1];
    return [$status, $text === '' ? null : json_decode($text, true)];  // {"code":…,"details":…}
}
// --- /README ---

if ($argc !== 2) {
    fwrite(STDERR, "usage: php sign.php <document.pdf>\n");
    exit(2);
}
$pdfPath = $argv[1];

[$status, $health] = liro_call('GET', '/v2/health');
printf("agent %s, protocol %d\n", $health['agentVersion'], $health['protocolVersion']);

$origin = getenv('LIRO_ORIGIN') ?: 'local';
$appId  = getenv('LIRO_APP_ID') ?: null;
$secret = getenv('LIRO_SECRET') ?: null;

if ($appId === null || $secret === null) {
    [$status, $req] = liro_call('POST', '/v2/pair/request',
        ['applicationName' => 'PHP primer', 'origin' => $origin]);
    if ($status !== 200) {
        fwrite(STDERR, "pair/request: " . json_encode($req) . "\n");
        exit(1);
    }
    echo "Liro Bridge is showing six digits on the screen.\n";
    echo 'Code: ';
    $code = trim((string) fgets(STDIN));

    // The same origin as the request above, from the same variable. A
    // confirm that declares a different one — or leaves it out — is
    // answered PAIRING_ORIGIN_MISMATCH.
    [$status, $ok] = liro_call('POST', '/v2/pair/confirm',
        ['requestId' => $req['requestId'], 'code' => $code, 'origin' => $origin]);
    if ($status !== 200) {
        fwrite(STDERR, "pair/confirm: " . json_encode($ok) . "\n");
        exit(1);
    }
    $appId  = $ok['appId'];
    $secret = $ok['deviceSecret'];
    echo "Keep these; the secret is returned once and never again:\n";
    echo "  set LIRO_APP_ID=$appId\n  set LIRO_SECRET=$secret\n";
}

[$status, $certs] = liro_call('GET', '/v2/certificates', null, $appId, $secret);
if ($status === 200) {
    foreach ($certs['certificates'] as $c) {
        printf("  %s  %s%s  %s\n", substr($c['thumbprint'], -8), $c['displayName'],
            $c['isTestKey'] ? ' [TEST KEY]' : '',
            $c['usable'] ? 'usable' : ($c['notUsableReason'] ?? ''));
    }
}

[$status, $job] = liro_call('POST', '/v2/sign/pdf', [
    'documents' => [['name' => basename($pdfPath), 'content' => base64_encode(file_get_contents($pdfPath))]],
    'level'     => 'b-b',
], $appId, $secret);
if ($status !== 202) {
    fwrite(STDERR, "sign/pdf: HTTP $status " . json_encode($job) . "\n");
    exit(1);
}
printf("job %s — approve it in the agent's window\nbatch fingerprint %s\n",
    $job['jobId'], $job['batchFingerprint']);

while (true) {
    sleep(1);
    [$status, $body] = liro_call('GET', $job['resultUrl'], null, $appId, $secret);
    if ($status === 202) {
        printf("  %s %d/%d\n", $body['state'], $body['completed'], $body['total']);
        continue;
    }
    if ($status !== 200) {
        fwrite(STDERR, "failed: HTTP $status " . json_encode($body) . "\n");
        exit(1);
    }
    $out = preg_replace('/\.pdf$/i', '', $pdfPath) . '-signed.pdf';
    file_put_contents($out, base64_decode($body['documents'][0]['content']));
    printf("saved %s at %s\n", $out, $body['documents'][0]['achievedLevel']);
    break;
}
