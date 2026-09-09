// Sign a PDF with Liro Bridge, from C#, with nothing but the base class
// library. Works on .NET Framework 4.5+ and on .NET 6+ unchanged.
//
//   csc /r:System.Net.Http.dll Sign.cs && Sign.exe ugovor.pdf
//   dotnet run ugovor.pdf
//
// Reads the pairing from LIRO_APP_ID and LIRO_SECRET when they are set,
// and pairs otherwise — the agent shows six digits on the person's
// screen and this asks for them.
//
// Written against docs/PROTOCOL.md. The four mistakes a first
// integration makes are avoided here by construction:
//
//   1. a wrong field name          — every name comes from §5.2's example
//   2. origin missing from confirm — ONE origin field, used in both calls
//   3. a body altered between being hashed and being sent — the bytes are
//      built once and both hashed and sent
//   4. a Content-Type on a request with no body — HttpClient puts the
//      header on the *content*, so a GET with no content has nowhere to
//      put one. That is not a limitation to work around: the agent does
//      not ask for one. This is the mistake that made every request from
//      a real .NET client fail once (PROTOCOL.md §2.5).

using System;
using System.IO;
using System.Net.Http;
using System.Net.Http.Headers;
using System.Security.Cryptography;
using System.Text;
using System.Text.RegularExpressions;
using System.Threading.Tasks;

internal static class Liro
{
    // --- README ---
    private static readonly HttpClient Http = new HttpClient();
    private static string _base;

    /// Reads the per-user discovery file. Never scan ports: several
    /// people can be signed in to one machine, each with their own agent.
    ///
    /// From the LOCALAPPDATA *environment variable*, because that is what
    /// the agent itself writes to. Environment.GetFolderPath reads the
    /// shell's known folder instead, and the two are not always the same
    /// path — under folder redirection, or in a process that inherited a
    /// different environment, they disagree and this looks in the wrong
    /// place. Measured: it did.
    private static void Discover()
    {
        var local = Environment.GetEnvironmentVariable("LOCALAPPDATA")
                    ?? Environment.GetFolderPath(Environment.SpecialFolder.LocalApplicationData);
        var path = Path.Combine(local, "Liro", "bridge.json");
        if (!File.Exists(path))
        {
            throw new FileNotFoundException("Liro Bridge is not running: no " + path, path);
        }
        var port = Regex.Match(File.ReadAllText(path), "\"port\"\\s*:\\s*(\\d+)").Groups[1].Value;
        _base = "http://127.0.0.1:" + port;
    }

    /// One request, signed when a secret is given.
    private static async Task<Tuple<int, string>> Call(
        string method, string path, string json, string appId, string secret)
    {
        var raw = json == null ? new byte[0] : Encoding.UTF8.GetBytes(json);  // hashed AND sent
        var request = new HttpRequestMessage(new HttpMethod(method), _base + path);
        if (raw.Length > 0)
        {
            request.Content = new ByteArrayContent(raw);                     // only when
            request.Content.Headers.ContentType =                            // there is a body
                new MediaTypeHeaderValue("application/json");
        }
        if (secret != null)
        {
            var ts = ((long)(DateTime.UtcNow - new DateTime(1970, 1, 1)).TotalSeconds).ToString();
            var nonce = Guid.NewGuid().ToString("N");
            string bodyHash;
            using (var sha = SHA256.Create()) bodyHash = Hex(sha.ComputeHash(raw));
            var canon = string.Join("\n", new[] { method.ToUpperInvariant(), path, ts, nonce, bodyHash });
            using (var mac = new HMACSHA256(Convert.FromBase64String(secret)))
            {
                request.Headers.Add("X-Liro-App-Id", appId);
                request.Headers.Add("X-Liro-Timestamp", ts);
                request.Headers.Add("X-Liro-Nonce", nonce);
                request.Headers.Add("X-Liro-Signature", Hex(mac.ComputeHash(Encoding.UTF8.GetBytes(canon))));
            }
        }
        var response = await Http.SendAsync(request);
        var body = await response.Content.ReadAsStringAsync();   // {"code":"...","details":{...}}
        return Tuple.Create((int)response.StatusCode, body);
    }

    private static string Hex(byte[] b) { return BitConverter.ToString(b).Replace("-", "").ToLowerInvariant(); }
    // --- /README ---

    /// The tiniest JSON reader that can read this protocol's answers: a
    /// real integration uses System.Text.Json or Newtonsoft, and this
    /// file has no dependencies on purpose.
    private static string Field(string json, string name)
    {
        var m = Regex.Match(json, "\"" + name + "\"\\s*:\\s*\"([^\"]*)\"");
        if (m.Success) return m.Groups[1].Value;
        m = Regex.Match(json, "\"" + name + "\"\\s*:\\s*([0-9]+)");
        return m.Success ? m.Groups[1].Value : null;
    }

    private static string Quote(string s) { return "\"" + s.Replace("\\", "\\\\").Replace("\"", "\\\"") + "\""; }

    // A synchronous entry point that waits on the asynchronous one.
    //
    // An "async Task<int> Main" is C# 7.1, and this file compiles with
    // the C# 5 compiler that ships inside every Windows installation, at
    // %WINDIR%/Microsoft.NET/Framework64/v4.0.30319/csc.exe. That is
    // deliberate: a Serbian ERP is as likely to be on .NET Framework as
    // on .NET 8, and an example that needs the newer one is an example
    // half the audience cannot run.
    private static int Main(string[] args)
    {
        return Run(args).GetAwaiter().GetResult();
    }

    private static async Task<int> Run(string[] args)
    {
        // A certificate's display name is a Serbian name, which is the
        // normal case here rather than an edge one, and a Windows console
        // runs on a legacy code page. Without this the names come out as
        // question marks.
        try { Console.OutputEncoding = Encoding.UTF8; } catch (IOException) { /* redirected */ }

        if (args.Length != 1) { Console.Error.WriteLine("usage: Sign.exe <document.pdf>"); return 2; }
        var pdfPath = args[0];
        Discover();

        var health = await Call("GET", "/v2/health", null, null, null);
        Console.WriteLine("health " + health.Item1 + " " + health.Item2);

        var origin = Environment.GetEnvironmentVariable("LIRO_ORIGIN") ?? "local";
        var appId = Environment.GetEnvironmentVariable("LIRO_APP_ID");
        var secret = Environment.GetEnvironmentVariable("LIRO_SECRET");

        if (string.IsNullOrEmpty(appId) || string.IsNullOrEmpty(secret))
        {
            var req = await Call("POST", "/v2/pair/request",
                "{\"applicationName\":" + Quote("C# primer") + ",\"origin\":" + Quote(origin) + "}", null, null);
            if (req.Item1 != 200) { Console.Error.WriteLine("pair/request: " + req.Item2); return 1; }
            var requestId = Field(req.Item2, "requestId");

            Console.WriteLine("Liro Bridge is showing six digits on the screen.");
            Console.Write("Code: ");
            var code = (Console.ReadLine() ?? "").Trim();

            // The same origin as the request above, from the same
            // variable. A confirm that declares a different one — or
            // leaves it out — is answered PAIRING_ORIGIN_MISMATCH.
            var ok = await Call("POST", "/v2/pair/confirm",
                "{\"requestId\":" + Quote(requestId) + ",\"code\":" + Quote(code) +
                ",\"origin\":" + Quote(origin) + "}", null, null);
            if (ok.Item1 != 200) { Console.Error.WriteLine("pair/confirm: " + ok.Item2); return 1; }

            appId = Field(ok.Item2, "appId");
            secret = Field(ok.Item2, "deviceSecret");
            Console.WriteLine("Keep these; the secret is returned once and never again:");
            Console.WriteLine("  set LIRO_APP_ID=" + appId);
            Console.WriteLine("  set LIRO_SECRET=" + secret);
        }

        var certs = await Call("GET", "/v2/certificates", null, appId, secret);
        Console.WriteLine("certificates " + certs.Item1 + " " + certs.Item2);

        var content = Convert.ToBase64String(File.ReadAllBytes(pdfPath));
        var submit = await Call("POST", "/v2/sign/pdf",
            "{\"documents\":[{\"name\":" + Quote(Path.GetFileName(pdfPath)) +
            ",\"content\":\"" + content + "\"}],\"level\":\"b-b\"}", appId, secret);
        if (submit.Item1 != 202) { Console.Error.WriteLine("sign/pdf: " + submit.Item2); return 1; }

        var resultUrl = Field(submit.Item2, "resultUrl");
        Console.WriteLine("job " + Field(submit.Item2, "jobId") + " — approve it in the agent's window");
        Console.WriteLine("batch fingerprint " + Field(submit.Item2, "batchFingerprint"));

        while (true)
        {
            await Task.Delay(1000);
            var poll = await Call("GET", resultUrl, null, appId, secret);
            if (poll.Item1 == 202)
            {
                Console.WriteLine("  " + Field(poll.Item2, "state") + " " +
                                  Field(poll.Item2, "completed") + "/" + Field(poll.Item2, "total"));
                continue;
            }
            if (poll.Item1 != 200) { Console.Error.WriteLine("failed: " + poll.Item1 + " " + poll.Item2); return 1; }

            var signed = Regex.Match(poll.Item2, "\"content\"\\s*:\\s*\"([^\"]*)\"").Groups[1].Value;
            var outPath = Path.ChangeExtension(pdfPath, null) + "-signed.pdf";
            File.WriteAllBytes(outPath, Convert.FromBase64String(signed));
            Console.WriteLine("saved " + outPath + " at " + Field(poll.Item2, "achievedLevel"));
            return 0;
        }
    }
}
