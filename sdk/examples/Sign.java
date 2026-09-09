// Sign a PDF with Liro Bridge, from Java, with nothing but the JDK.
//
//   java Sign.java ugovor.pdf          (JDK 11 or later, single-file mode)
//
// Reads the pairing from LIRO_APP_ID and LIRO_SECRET when they are set,
// and pairs otherwise — the agent shows six digits on the person's
// screen and this asks for them.
//
// Written against docs/PROTOCOL.md. The four mistakes a first
// integration makes are avoided here by construction:
//
//   1. a wrong field name          — every name comes from §5.2's example
//   2. origin missing from confirm — ONE origin variable, used in both calls
//   3. a body altered between being hashed and being sent — the bytes are
//      built once and both hashed and sent
//   4. a Content-Type on a request with no body — only set when there is one

import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.nio.file.Files;
import java.nio.file.Path;
import java.nio.file.Paths;
import java.security.MessageDigest;
import java.util.Base64;
import java.util.Scanner;
import java.util.UUID;
import java.util.regex.Matcher;
import java.util.regex.Pattern;
import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;

public final class Sign {

    // --- README ---
    private static final HttpClient HTTP = HttpClient.newHttpClient();
    private static String base;

    /** Reads the per-user discovery file. Never scan ports. */
    static void discover() throws IOException {
        Path file = Paths.get(System.getenv("LOCALAPPDATA"), "Liro", "bridge.json");
        Matcher m = Pattern.compile("\"port\"\\s*:\\s*(\\d+)").matcher(Files.readString(file));
        if (!m.find()) throw new IOException("no port in " + file);
        base = "http://127.0.0.1:" + m.group(1);
    }

    /** One request, signed when a secret is given. Returns {status, body}. */
    static Object[] call(String method, String path, String json, String appId, String secret)
            throws Exception {
        byte[] raw = json == null ? new byte[0] : json.getBytes(StandardCharsets.UTF_8); // hashed AND sent
        HttpRequest.Builder b = HttpRequest.newBuilder(URI.create(base + path))
                .method(method, raw.length == 0
                        ? HttpRequest.BodyPublishers.noBody()
                        : HttpRequest.BodyPublishers.ofByteArray(raw));
        if (raw.length > 0) b.header("Content-Type", "application/json");  // only with a body
        if (secret != null) {
            String ts = Long.toString(System.currentTimeMillis() / 1000);
            String nonce = UUID.randomUUID().toString().replace("-", "");
            String bodyHash = hex(MessageDigest.getInstance("SHA-256").digest(raw));
            String canon = String.join("\n", method.toUpperCase(), path, ts, nonce, bodyHash);
            Mac mac = Mac.getInstance("HmacSHA256");
            mac.init(new SecretKeySpec(Base64.getDecoder().decode(secret), "HmacSHA256"));
            b.header("X-Liro-App-Id", appId).header("X-Liro-Timestamp", ts).header("X-Liro-Nonce", nonce)
             .header("X-Liro-Signature", hex(mac.doFinal(canon.getBytes(StandardCharsets.UTF_8))));
        }
        HttpResponse<String> r = HTTP.send(b.build(), HttpResponse.BodyHandlers.ofString());
        return new Object[] {r.statusCode(), r.body()};   // {"code":"...","details":{...}}
    }

    static String hex(byte[] bytes) {
        StringBuilder s = new StringBuilder(bytes.length * 2);
        for (byte x : bytes) s.append(String.format("%02x", x));
        return s.toString();
    }
    // --- /README ---

    /** The smallest JSON reader that can read this protocol's answers. */
    static String field(String json, String name) {
        Matcher m = Pattern.compile("\"" + name + "\"\\s*:\\s*\"([^\"]*)\"").matcher(json);
        if (m.find()) return m.group(1);
        m = Pattern.compile("\"" + name + "\"\\s*:\\s*(\\d+)").matcher(json);
        return m.find() ? m.group(1) : null;
    }

    static String quote(String s) {
        return "\"" + s.replace("\\", "\\\\").replace("\"", "\\\"") + "\"";
    }

    public static void main(String[] args) throws Exception {
        // A certificate's display name is a Serbian name, which is the
        // normal case here rather than an edge one, and a Windows console
        // runs on a legacy code page. Without this the names come out as
        // question marks.
        System.setOut(new java.io.PrintStream(new java.io.FileOutputStream(java.io.FileDescriptor.out),
                true, StandardCharsets.UTF_8));

        if (args.length != 1) {
            System.err.println("usage: java Sign.java <document.pdf>");
            System.exit(2);
        }
        Path pdf = Paths.get(args[0]);
        discover();

        Object[] health = call("GET", "/v2/health", null, null, null);
        System.out.println("health " + health[0] + " " + health[1]);

        String origin = System.getenv("LIRO_ORIGIN") != null ? System.getenv("LIRO_ORIGIN") : "local";
        String appId = System.getenv("LIRO_APP_ID");
        String secret = System.getenv("LIRO_SECRET");

        if (appId == null || secret == null) {
            Object[] req = call("POST", "/v2/pair/request",
                    "{\"applicationName\":" + quote("Java primer") + ",\"origin\":" + quote(origin) + "}",
                    null, null);
            if ((int) req[0] != 200) { System.err.println("pair/request: " + req[1]); System.exit(1); }
            String requestId = field((String) req[1], "requestId");

            System.out.println("Liro Bridge is showing six digits on the screen.");
            System.out.print("Code: ");
            String code = new Scanner(System.in).nextLine().trim();

            // The same origin as the request above, from the same
            // variable. A confirm that declares a different one — or
            // leaves it out — is answered PAIRING_ORIGIN_MISMATCH.
            Object[] ok = call("POST", "/v2/pair/confirm",
                    "{\"requestId\":" + quote(requestId) + ",\"code\":" + quote(code)
                            + ",\"origin\":" + quote(origin) + "}", null, null);
            if ((int) ok[0] != 200) { System.err.println("pair/confirm: " + ok[1]); System.exit(1); }

            appId = field((String) ok[1], "appId");
            secret = field((String) ok[1], "deviceSecret");
            System.out.println("Keep these; the secret is returned once and never again:");
            System.out.println("  set LIRO_APP_ID=" + appId);
            System.out.println("  set LIRO_SECRET=" + secret);
        }

        Object[] certs = call("GET", "/v2/certificates", null, appId, secret);
        System.out.println("certificates " + certs[0] + " " + certs[1]);

        String content = Base64.getEncoder().encodeToString(Files.readAllBytes(pdf));
        Object[] submit = call("POST", "/v2/sign/pdf",
                "{\"documents\":[{\"name\":" + quote(pdf.getFileName().toString())
                        + ",\"content\":\"" + content + "\"}],\"level\":\"b-b\"}", appId, secret);
        if ((int) submit[0] != 202) { System.err.println("sign/pdf: " + submit[1]); System.exit(1); }

        String resultUrl = field((String) submit[1], "resultUrl");
        System.out.println("job " + field((String) submit[1], "jobId") + " — approve it in the agent's window");
        System.out.println("batch fingerprint " + field((String) submit[1], "batchFingerprint"));

        while (true) {
            Thread.sleep(1000);
            Object[] poll = call("GET", resultUrl, null, appId, secret);
            if ((int) poll[0] == 202) {
                System.out.println("  " + field((String) poll[1], "state") + " "
                        + field((String) poll[1], "completed") + "/" + field((String) poll[1], "total"));
                continue;
            }
            if ((int) poll[0] != 200) {
                System.err.println("failed: " + poll[0] + " " + poll[1]);
                System.exit(1);
            }
            String signed = field((String) poll[1], "content");
            Path out = pdf.resolveSibling(pdf.getFileName().toString().replaceFirst("\\.pdf$", "") + "-signed.pdf");
            Files.write(out, Base64.getDecoder().decode(signed));
            System.out.println("saved " + out + " at " + field((String) poll[1], "achievedLevel"));
            return;
        }
    }
}
