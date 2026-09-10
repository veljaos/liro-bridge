Liro Bridge, potpisivanje PDF dokumenata kvalifikovanim elektronskim
sertifikatom sa kartice ili USB tokena.

**Šta da preuzmete**

| Fajl | Za koga |
|---|---|
| `liro-bridge-<verzija>-x64.msi` | **Većina korisnika.** Instalira se bez administratorskih prava. |
| `liro-bridge-<verzija>-x64-per-machine.msi` | Za administratore koji instaliraju preko Group Policy. Traži administratorska prava. |
| `liro-bridge-<verzija>-x64.exe` | Sam program, bez instalacije. |
| `release.json`, `release.json.sig` | Potpisani opis izdanja. Program ih koristi kada proverava ima li novije verzije. |

**Pri prvom pokretanju Windows će prikazati upozorenje** ("Windows
protected your PC"). Program još nema Authenticode sertifikat. Kliknite
**More info**, pa **Run anyway**. Uputstvo objašnjava zašto, sa slikom:
`docs/guide/Uputstvo.html`, ili prečica *Liro Bridge — uputstvo* u Start
meniju posle instalacije.

**Šta vam je potrebno**

- Windows 10 ili 11, 64-bitni.
- Microsoft Edge WebView2 Runtime. Windows 11 ga već ima; ako nedostaje,
  instalacija će to reći i dati adresu.
- Čitač kartica i middleware vašeg izdavača sertifikata (MUP, Pošta
  Srbije, Halcom).

---

🤖 Generated with [Claude Code](https://claude.com/claude-code)
