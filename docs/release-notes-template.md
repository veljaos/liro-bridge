Liro Bridge, potpisivanje PDF dokumenata kvalifikovanim elektronskim
sertifikatom sa kartice ili USB tokena.

**Šta da preuzmete**

Fajlovi su niže na ovoj stranici, pod *Assets*. Ime svakog sadrži broj
verzije ovog izdanja.

| Fajl | Za koga |
|---|---|
| `.msi` — onaj **bez** `-per-machine` u imenu | **Većina korisnika.** Instalira se bez administratorskih prava. |
| `.msi` — onaj **sa** `-per-machine` u imenu | Za administratore koji instaliraju preko Group Policy. Traži administratorska prava. |
| `.exe` | Sam program, bez instalacije. |
| `release.json`, `release.json.sig` | Potpisani opis izdanja. Program ih koristi kada proverava ima li novije verzije; nije potrebno da ih preuzimate. |

**Pri prvom pokretanju Windows će prikazati upozorenje** ("Windows
protected your PC"). Program još nema Authenticode sertifikat. Kliknite
**More info**, pa **Run anyway**. Uputstvo objašnjava zašto, sa slikom:
prečica *Liro Bridge — uputstvo* u Start meniju posle instalacije, ili
`Uputstvo.html` u folderu u koji je program instaliran. Isto uputstvo na
engleskom je `Guide.html`, pored njega.

**Šta vam je potrebno**

- Windows 10 ili 11, 64-bitni.
- Microsoft Edge WebView2 Runtime. Windows 11 ga već ima; ako nedostaje,
  instalacija će to reći i dati adresu.
- Čitač kartica i middleware vašeg izdavača sertifikata (MUP, Pošta
  Srbije, Halcom).
