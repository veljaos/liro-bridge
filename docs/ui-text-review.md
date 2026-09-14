# Text review — every string a user sees

The source is `internal/i18n/locales/sr-Latn.json`, in full: all 365 keys,
nothing omitted. The two guides, `docs/guide/Uputstvo.html` and
`docs/guide/Guide.html`, are deliberately not here.

Read it top to bottom and write on it. Once the wording is settled, the
same changes go into `en.json` and `sr-Cyrl.json`.

> **This document describes the catalogue as it stood at 609a5fc, before
> the text pass.** The pass has since changed about 50 strings, deleted
> six keys and added two, so a sentence quoted here is not necessarily
> the sentence the program says today — a reading of demo B was misled
> by exactly that once (see C3). The tables are kept as written, because
> what they are for is the record of what was surveyed and why; where a
> finding has been settled, the finding says so underneath itself.

**How it is ordered.** By where the text appears in the running program,
not by the JSON file's alphabetical order: the signing window's three
steps first, in the order a person meets them, then the windows the tray
opens, then pairing, then the update prompt, then the tray and the
Explorer verb, then the error messages, and last the command line.

**How to read the columns.** *Key* is the catalogue key, so any line can
be pointed at unambiguously. *Current text* is verbatim from the file —
`%s`, `%d` and `%.0f` are values filled in at runtime, and `<br>` marks a
line break inside the string. *Where it appears* names the window and the
moment; a string used in more than one place says so in **bold**.

Findings — text that does not match what the program does, and wording
that differs between two places meaning the same thing — are collected at
the end, after the tables. The Explorer verb you already want shortened is
the section after that.

## Contents

- [1. Signing window — the window itself, and the step header](#1-signing-window--the-window-itself-and-the-step-header) — 4 strings
- [2. Signing window — step 1, which documents](#2-signing-window--step-1-which-documents) — 21 strings
- [3. Signing window — step 2, which certificate](#3-signing-window--step-2-which-certificate) — 19 strings
- [4. Signing window — step 3, how to sign](#4-signing-window--step-3-how-to-sign) — 26 strings
- [5. Placement picker (opens from step 3)](#5-placement-picker-opens-from-step-3) — 17 strings
- [6. Signing window — questions asked after the approval, before the card](#6-signing-window--questions-asked-after-the-approval-before-the-card) — 21 strings
- [7. Signing window — while the batch runs](#7-signing-window--while-the-batch-runs) — 11 strings
- [8. Signing window — the report](#8-signing-window--the-report) — 28 strings
- [9. Settings](#9-settings) — 66 strings
- [10. Certificates window](#10-certificates-window) — 3 strings
- [11. Audit log window](#11-audit-log-window) — 10 strings
- [12. Pairing window](#12-pairing-window) — 9 strings
- [13. Update prompt](#13-update-prompt) — 11 strings
- [14. Tray menu, the Explorer verb, and install-time notices](#14-tray-menu-the-explorer-verb-and-install-time-notices) — 13 strings
- [15. Error messages](#15-error-messages) — 40 strings
- [16. Command-line output](#16-command-line-output) — 66 strings
- [Findings](#findings)
- [The Explorer verb](#the-explorer-verb)


## 1. Signing window — the window itself, and the step header

One window (`main.title`) whose content changes across steps. The step header is absent when the run has only one step.

| Key | Current text | Where it appears |
|---|---|---|
| `main.title` | Liro Bridge | Title bar, every step and every state of the signing window. Also the title while the report is on screen. |
| `step.counter` | Korak %d od %d | Step header, as the screen-reader label for the row of dots. Never drawn as text. |
| `step.back` | Nazad | Step header, the button back to the previous step. Hidden on the first step. |
| `step.next` | Dalje | Primary button, on every step except the last. On the last step it says `stampwindow.sign` instead. |

## 2. Signing window — step 1, which documents

`main.html` → `#state-files`. The drop zone, the file list, where output goes, and the button that moves on.

| Key | Current text | Where it appears |
|---|---|---|
| `main.empty_title` | Prevucite PDF dokumente ovde | Drop zone heading, shown until the first document arrives. |
| `main.empty_hint` | Ili ih izaberite dugmetom Izaberi. Folder donosi PDF-ove koji su u njemu. | Drop zone, under the heading. Names the Izaberi button. |
| `main.document_count_one` | 1 dokument, %s | Count line above the list, exactly one document. `%s` is the total size. |
| `main.document_count` | %d dokumenata, %s | Count line above the list, two or more documents. `%d` count, `%s` total size. |
| `main.size_unknown` | veličina nepoznata | Stands in for a size that could not be read — alone as the whole count line, or appended to it. |
| `main.remove_file` | Ukloni | The button on each row of the file list. |
| `main.notice_folder_scanned` | %s: dodato %d PDF dokumenata. | Notice above the list after a folder is dropped. `%s` folder name, `%d` how many PDFs it held. |
| `main.notice_folder_empty` | %s ne sadrži nijedan PDF dokument. | Notice above the list, styled as a problem, when a dropped folder held no PDF. |
| `main.notice_duplicate` | %s je već na spisku. | Notice above the list when a document already in the queue is added again. |
| `main.notice_unreadable` | %s ne može da se pročita. | Notice above the list, styled as a problem, when a dropped file could not be read. |
| `main.output_label` | Sačuvaj potpisane u | Label of the output row in the footer. |
| `main.output_beside_input` | isti folder u kome je dokument | The *value* of that row when no folder has been chosen. |
| `main.output_beside` | Pored svakog dokumenta | The button that gives that setting back, shown only once a folder has been chosen. |
| `main.output_change` | Promeni... | The button that opens the folder chooser. |
| `main.choose_output_folder` | Izaberite gde se čuvaju potpisani dokumenti | Title bar of the Windows folder chooser that button opens. |
| `main.clear` | Ukloni sve | Footer, the quiet button that empties the list. |
| `main.browse` | Izaberi... | Footer, the button that opens the file chooser. |
| `main.choose_files` | Izaberite PDF dokumente | Title bar of the Windows file chooser that button opens. |
| `main.file_filter_pdf` | PDF dokumenti | The two file-type filters in that chooser. **Also** used by the chooser the stamp window opens when Settings needs a document to place a stamp on. |
| `main.file_filter_all` | Sve datoteke | The two file-type filters in that chooser. **Also** used by the chooser the stamp window opens when Settings needs a document to place a stamp on. |
| `main.sign_opening` | Otvaranje… | Replaces the primary button's own label while the next step is being prepared. |

## 3. Signing window — step 2, which certificate

`consent.html`. Who is asking, how many documents, the certificate list, and the approval.

| Key | Current text | Where it appears |
|---|---|---|
| `consent.window_title` | Odobrite potpis | **Unused.** Nothing sets a window title here — the window keeps `main.title` on every step. Referenced only by tests. |
| `consent.document_count` | %d dokument(a) za potpisivanje | Under the signer's name: how many documents this approval covers. |
| `consent.select_certificate_prompt` | Izaberite sertifikat da biste nastavili. | Above the certificate list, when there is something to choose. |
| `consent.looking_for_certificates` | Tražim vaš sertifikat… | Replaces that prompt while the card is still being read. |
| `consent.no_certificate_found` | Na ovoj kartici nije pronađen sertifikat za potpisivanje. | Replaces that prompt when the listing finished and nothing on the card can sign. |
| `consent.role_signing` | za potpisivanje | On each certificate row, after the name. **Also** on every row of the Certificates window. |
| `consent.role_authentication` | za prijavu | On each certificate row, after the name. **Also** on every row of the Certificates window. |
| `consent.role_unknown` | nepoznata namena | On each certificate row, after the name. **Also** on every row of the Certificates window. |
| `certs.test_key_marker` | (TEST KLJUČ) | Badge on a certificate row backed by the soft token. **Also** in the Certificates window, the audit log, and the `certificates` command. |
| `consent.application_label` | Aplikacija | Footer label for who is asking. |
| `consent.local_application` | Ovaj računar | The *value* of that row when the batch was started on this machine rather than by a paired application. |
| `consent.details_toggle` | Detalji | The disclosure that hides the fingerprint and the file list. |
| `consent.fingerprint_label` | Otisak serije | Inside Details, label of the batch fingerprint. |
| `consent.copy_fingerprint` | Kopiraj | Inside Details, the button beside the fingerprint. |
| `consent.files_label` | Fajlovi | Inside Details, heading of the file list. |
| `consent.files_overflow` | ...i još %d | Inside Details, last line of a file list too long to show in full. |
| `consent.countdown` | Ovaj zahtev ističe za %d s. | Warning line above the buttons, only for a request that arrived over the protocol, only in its last thirty seconds. |
| `consent.cancel` | Otkaži | The secondary button. **Also** the third button on all three of the questions in group 6. |
| `consent.approve` | Odobri | The primary button. Disabled until a certificate is selected. |

## 4. Signing window — step 3, how to sign

`stamp.html`. Three methods; the extra fields marked *(Settings only)* appear only when this same window is opened from Settings' **Vidljivi pečat…** button, not during signing.

| Key | Current text | Where it appears |
|---|---|---|
| `stampwindow.title` | Metod potpisivanja | Heading of the method screen. **Also** the title bar when this window is opened from Settings. |
| `stampwindow.method_placed` | Potpiši birajući poziciju potpisa | First method: open the picker and put the stamp where you choose. |
| `stampwindow.method_corners` | Potpiši sa definisanim pozicijama | Second method: one of four corners. |
| `stampwindow.method_none` | Potpiši bez vizuelnog prikaza | Third method: no visible mark on the page. |
| `stampwindow.position_label` | Ugao | Screen-reader label of the two-by-two corner grid under the second method. Never drawn as text. |
| `stampwindow.position_top_left` | Gore levo | The four buttons of that grid, laid out as they sit on a page. |
| `stampwindow.position_top_right` | Gore desno | The four buttons of that grid, laid out as they sit on a page. |
| `stampwindow.position_bottom_left` | Dole levo | The four buttons of that grid, laid out as they sit on a page. |
| `stampwindow.position_bottom_right` | Dole desno | The four buttons of that grid, laid out as they sit on a page. |
| `stampwindow.place_button` | Postavi na stranu… | Under the first method, the button that opens the placement picker. |
| `stampwindow.placed_reset` | Vrati na ugao | Beside it, the button that gives up a remembered position and goes back to corners. |
| `stampwindow.placed_at` | Sačuvano: strana %d, x %.0f, y %.0f | The remembered position, in words. Shown in the placement picker's footer, not here. `%d` page, then x and y in points. |
| `stampwindow.sign` | Potpiši | The primary button on this screen when it is the last step. **Also** `step.next`'s replacement on any last step. |
| `stampwindow.cancel` | Odustani | The secondary button, in both roles. |
| `stampwindow.subtitle_settings` | Kako se vaš potpis crta, dok ovo ne promenite. | *(Settings only)* The line under the heading. Absent when this is a step of signing. |
| `stampwindow.save` | Sačuvaj | *(Settings only)* The primary button. Replaced by `stampwindow.sign` when this is a step. |
| `stampwindow.page_label` | Strana | *(Settings only)* Label of the page dropdown. |
| `stampwindow.page_first` | Prva strana | *(Settings only)* The three entries of that dropdown. |
| `stampwindow.page_last` | Poslednja strana | *(Settings only)* The three entries of that dropdown. |
| `stampwindow.page_number` | Određena strana | *(Settings only)* The three entries of that dropdown. |
| `stampwindow.page_number_label` | Broj strane | *(Settings only)* Label of the number box that appears when the third entry is chosen. |
| `stampwindow.reference_label` | Referenca (nije obavezno) | *(Settings only)* Label of the free-text reference field. |
| `stampwindow.reference_hint` | Vaša oznaka za ovaj dokument, ispisana na pečatu. | *(Settings only)* Hint under it. |
| `stampwindow.show_document_id` | Prikaži broj ličnog dokumenta | *(Settings only)* Checkbox. |
| `stampwindow.show_document_id_hint` | Lični podatak na dokumentu koji drugi čitaju. Isključeno osim ako vam treba. | *(Settings only)* Hint under it. |
| `stampwindow.margin_note` | Pečat uvek ostavlja 12 pt slobodno od svake ivice strane. | *(Settings only)* Last line of the form. The 12 pt is `appearance.Margin` and is correct. |

## 5. Placement picker (opens from step 3)

`place.html`. A window of its own — the only thing that opens on top of the signing window. Reached by choosing the first method and pressing the primary button.

| Key | Current text | Where it appears |
|---|---|---|
| `place.title` | Pozicija potpisa | Title bar of the picker. **Also**, wrongly, the title bar of the *file chooser* Settings opens first when it has no document to draw on — see finding F6. |
| `place.first` | Prva | The four page-navigation buttons in the top bar, around the page number box. |
| `place.prev` | Prethodna | The four page-navigation buttons in the top bar, around the page number box. |
| `place.next` | Sledeća | The four page-navigation buttons in the top bar, around the page number box. |
| `place.last` | Poslednja | The four page-navigation buttons in the top bar, around the page number box. |
| `place.of` | od | After the page number box: `od 12`. |
| `place.fit` | Uklopi | The zoom button's label when the page is scaled to fit; otherwise it shows a percentage. |
| `place.snapped_to` | Prionulo uz | Footer, while the stamp is snapped to an edge. Followed by one of the four `place.corner_*` strings. |
| `place.corner_bottom_left` | dole levo | Completes that line. Not used anywhere else — the corner *buttons* use `stampwindow.position_*`. |
| `place.corner_bottom_right` | dole desno | Completes that line. Not used anywhere else — the corner *buttons* use `stampwindow.position_*`. |
| `place.corner_top_left` | gore levo | Completes that line. Not used anywhere else — the corner *buttons* use `stampwindow.position_*`. |
| `place.corner_top_right` | gore desno | Completes that line. Not used anywhere else — the corner *buttons* use `stampwindow.position_*`. |
| `place.reset_to_saved` | Vrati na sačuvanu | Footer, the button back to the position this document was last signed at. |
| `place.use` | Koristi ovu poziciju | Footer, the primary button. |
| `place.cancel` | Odustani | Footer, the secondary button. |
| `place.preview_partial` | Ova strana je previše složena da bi bila iscrtana u celosti, pa pregled prikazuje njen deo. Pečat i dalje ide tačno tamo gde ga postavite. | Note drawn over the page when the renderer could not draw all of it. |
| `place.unavailable` | Ovaj dokument ne može da se prikaže, pa se pečat postavlja po uglu. | Status line on the *method* screen when a document **on this machine** cannot be drawn by the renderer, so the picker never opens. Since the text pass this is its only job — `stampwindow_windows.go` is the one producer. |
| `place.unavailable_no_file` | Ovi dokumenti su stigli od aplikacije i nisu datoteke na ovom računaru, pa nema strane koja bi se prikazala. Pečat se postavlja po uglu. | The same status line for a batch that arrived over the protocol, where there is no file to draw at all. Added by the text pass — see C3. |

## 6. Signing window — questions asked after the approval, before the card

`main.html` → `#state-tsachoice`, `#state-outputexists`, `#state-alreadysigned`, `#state-failed`. Each replaces the window's content; none is a separate window.

| Key | Current text | Where it appears |
|---|---|---|
| `consent.tsa_choice_title` | Bez vremenskog žiga | Heading of the timestamp question. |
| `consent.tsa_reason_not_configured` | Nijedan izdavalac vremenskog žiga nije podešen. | Under it, when no timestamp authority has been set. |
| `consent.tsa_reason_unreachable` | Podešeni izdavalac vremenskog žiga nije odgovorio. | Under it, when the configured one did not answer. |
| `consent.tsa_choice_explain` | Potpis bez vremenskog žiga je nivoa B-B: važeći, ali bez dokaza o tome kada je napravljen, što postaje bitno kada sertifikat istekne ili bude opozvan. | The paragraph under the reason. |
| `consent.tsa_save_without_timestamp` | Potpiši bez vremenskog žiga | Primary button. |
| `consent.tsa_configure` | Podesi izdavaoca vremenskog žiga | Secondary button; opens Settings. |
| `consent.output_exists_title` | Fajl već postoji | Heading of the existing-file question. |
| `consent.output_exists_explain` | Ništa nije prepisano. Izaberite šta da se uradi sa potpisanim dokumentom. | The paragraph under it. |
| `consent.output_exists_path_label` | Postojeći fajl | Label of the path shown above the buttons. |
| `consent.output_exists_rename` | Sačuvaj kao %s | Primary button. `%s` is the file name it would use instead. |
| `consent.output_exists_overwrite` | Prepiši | Secondary button. |
| `consent.already_signed_title` | Neki od ovih su već potpisani | Heading of the already-signed question. |
| `consent.already_signed_explain_one` | 1 od ovih dokumenata je već potpisan dokument: ime se završava na %s, pa bi potpisivanje dodalo drugi potpis preko prvog. | The paragraph under it. `%s` is the configured output suffix. |
| `consent.already_signed_explain_many` | %d od ovih dokumenata su već potpisani dokumenti: imena se završavaju na %s, pa bi potpisivanje dodalo drugi potpis preko prvog. | The paragraph under it. `%s` is the configured output suffix. |
| `consent.already_signed_skip_one` | Preskoči ga i potpiši ostale | Primary button. |
| `consent.already_signed_skip_many` | Preskoči ih i potpiši ostale | Primary button. |
| `consent.already_signed_sign_one` | Potpiši i njega | Secondary button. |
| `consent.already_signed_sign_many` | Potpiši i njih | Secondary button. |
| `consent.state_failed` | Nešto nije u redu | Heading of the screen for a batch that failed before a single document was signed. |
| `consent.copy_technical_details` | Kopiraj tehničke detalje | Secondary button there. |
| `consent.close` | Zatvori | Primary button there. |

## 7. Signing window — while the batch runs

`main.html` → `#state-queue`.

| Key | Current text | Where it appears |
|---|---|---|
| `consent.state_preparing_card` | Priprema kartice... | Progress heading before the first signature. |
| `consent.state_signing` | Potpisivanje %d od %d | Progress heading thereafter: `%d` of `%d`. |
| `consent.eta_label` | Preostaje otprilike %s | Under the progress bar. `%s` is a duration already formatted in Go. |
| `consent.per_signature_pin_warning` | Ova kartica će tražiti PIN pre svakog potpisa. | Under the ETA, on a card that asks for the PIN every time. |
| `main.state_waiting` | čeka | The state on each row of the queue list. |
| `main.state_signing` | potpisuje se | The state on each row of the queue list. |
| `main.state_done` | potpisan | The state on each row of the queue list. |
| `main.state_failed` | neuspešno | The state on each row of the queue list. |
| `main.state_skipped` | preskočen | The state on each row of the queue list. |
| `main.stop` | Zaustavi | The only button on this screen. |
| `main.stopping` | Završava se tekući dokument... | Replaces that button's label after it is pressed, while the document in flight finishes. |

## 8. Signing window — the report

`main.html` → `#state-report`. Shown when the batch ends, however it ended.

| Key | Current text | Where it appears |
|---|---|---|
| `main.report_title` | Završeno | Report heading after a batch that ran to the end. **Also** the first line of the exported report file. |
| `main.report_stopped_title` | Zaustavljeno | Report heading after a batch that was stopped. |
| `main.report_succeeded` | %d potpisano | The counts line under the heading, joined together. Only the non-zero parts appear. |
| `main.report_failed` | %d neuspešno | The counts line under the heading, joined together. Only the non-zero parts appear. |
| `main.report_skipped` | %d nije pokušano | The counts line under the heading, joined together. Only the non-zero parts appear. |
| `main.aborted_card` | Potpisivanje je prekinuto: kartica više nije dostupna. | Error box under the counts when the card went away mid-batch. |
| `main.aborted_pin` | Potpisivanje je prekinuto: kartica je blokirana i traži PUK. | Error box under the counts when the card blocked mid-batch. |
| `sign.stamp_adjusted_one` | Pečat je pomeren da bi stao na jedan dokument. | Quiet note under the counts when a remembered stamp position had to be moved to fit. |
| `sign.stamp_adjusted_many` | Pečat je pomeren da bi stao na %d dokumenata. | Quiet note under the counts when a remembered stamp position had to be moved to fit. |
| `main.report_already_signed_one` | 1 dokument je preskočen jer je već potpisan. | Quiet note under the counts: how many were left alone for already carrying the suffix. |
| `main.report_already_signed_many` | Preskočeno dokumenata jer su već potpisani: %d. | Quiet note under the counts: how many were left alone for already carrying the suffix. |
| `main.report_output_collision_one` | Jedan dokument nije potpisan: njegov potpis bi zamenio drugi dokument iz ove grupe. | Caution note under the counts: documents whose output would have overwritten another input in the same batch. |
| `main.report_output_collision_many` | %d dokumenata nije potpisano: njihovi potpisi bi zamenili druge dokumente iz ove grupe. | Caution note under the counts: documents whose output would have overwritten another input in the same batch. |
| `audit.chain_continued` | Dnevnik revizije nije mogao da se nastavi u %s, pa je nastavljen u novom fajlu, %s, u %s. Stari fajl je ostavljen tačno onakav kakav je bio. | Caution note under the counts when the audit log had to start a new file. `%s` old file, `%s` new file, `%s` folder. |
| `audit.chain_continued_here` | Dnevnik revizije nije mogao da se nastavi u %s, pa je nastavljen u novom fajlu u %s. Stari fajl je ostavljen tačno onakav kakav je bio. | The same note when the new file is in the same folder, so the folder is named once. |
| `main.report_failures_title` | Šta nije uspelo | Heading of the list of failures in the scrolling body. **Also** a heading in the exported report file. |
| `main.report_output_label` | Sačuvano u | Footer label for where the signed documents went. **Also** in the exported report file. |
| `main.report_output_various` | folder svakog dokumenta | The *value* of that row when they went beside their inputs. |
| `main.report_output_returned` | vraćeno aplikaciji koja je tražila | The *value* of that row when they went back to the application that asked. |
| `main.report_level_label` | Nivo potpisa | Footer label for the level actually reached. **Also** in the exported report file and in Settings, as `settings.signature_level_label`, with different wording — see finding F9. |
| `consent.level_bb` | Nivo B-B — bez vremenskog žiga | The warning line under that row, only when the batch came out at B-B. |
| `main.export_report` | Sačuvaj izveštaj... | Footer, the quiet button. |
| `main.export_report_title` | Izaberite gde se čuva izveštaj | Title bar of the folder chooser it opens. |
| `main.export_report_done` | Izveštaj sačuvan u %s | Status line after it wrote the file. `%s` is the folder. |
| `main.export_report_failed` | Izveštaj nije mogao da se sačuva. | Status line when it could not. |
| `main.open_output` | Otvori folder | Footer, opens Explorer on the output folder. |
| `main.new_batch` | Potpiši još dokumenata | Footer, second row, goes back to step 1. |
| `main.finish` | Završi | Footer, second row, the primary button; closes the window. |

## 9. Settings

`settings.html`, opened from the tray. Listed in on-screen order.

| Key | Current text | Where it appears |
|---|---|---|
| `settings.window_title` | Podešavanja | Heading and title bar. |
| `settings.language_label` | Jezik | The language dropdown. Its three entries are hard-coded in `settings.html`, not translated. |
| `settings.start_with_windows` | Pokreni sa Windows-om | Checkbox. |
| `settings.tsa_label` | Servis za vremenske žigove | Legend of the timestamp-authority fieldset. |
| `settings.tsa_preset_freetsa` | freetsa.org | First preset radio. |
| `settings.tsa_preset_freetsa_warning` | nije kvalifikovan u Srbiji | Warning badge beside it. |
| `settings.tsa_preset_rsgov` | Kancelarija za IT i eUpravu (RS-GOV TSA) | Second preset radio. |
| `settings.tsa_preset_rsgov_note` | Zahteva odobren zahtev Kancelariji i klijentski sertifikat. | Note under it. |
| `settings.tsa_url_label` | URL | The URL field, inside the same fieldset. |
| `settings.tsa_custom_url_placeholder` | Prilagođena adresa | Its placeholder. |
| `settings.tsa_user_label` | Korisničko ime za servis za vremenske žigove | Username field. |
| `settings.tsa_password_label` | Lozinka za servis za vremenske žigove | Password field. |
| `settings.tsa_client_cert_label` | Klijentski sertifikat za servis za vremenske žigove (PKCS#12) | Client-certificate path field. |
| `settings.tsa_client_cert_placeholder` | Putanja do .p12/.pfx fajla | Its placeholder. |
| `settings.tsa_client_cert_password_label` | Lozinka klijentskog sertifikata za servis za vremenske žigove | Client-certificate password field. |
| `settings.output_suffix_label` | Sufiks izlaznog fajla | Suffix field. |
| `settings.output_folder_label` | Sačuvaj potpisane dokumente u | Output folder field. |
| `settings.output_folder_default` | Ostavite prazno da se čuva pored svakog dokumenta. | Hint under it. |
| `settings.explorer_menu` | Dodaj "Potpiši koristeći Liro Bridge" u meni desnog klika na PDF datotekama | Checkbox for the right-click entry. **Quotes the verb**, so it has to change whenever `settings.explorer_menu_verb` does. |
| `settings.explorer_menu_failed` | Stavka menija desnog klika nije mogla da se promeni. | Status line when the registry entry could not be written or removed. |
| `settings.document_signing` | Dozvoli programima da šalju cele dokumente na potpisivanje | Checkbox. |
| `settings.document_signing_hint` | Kada je isključeno, programi i dalje mogu da traže potpis nad otiskom koji su sami izračunali — agent tada uopšte ne prima dokument. | Hint under it. |
| `settings.certificate_listing` | Dozvoli programima da pitaju koji su sertifikati na ovom računaru | Checkbox. |
| `settings.certificate_listing_hint` | Program koji je povezan tada može da pročita imena sa sertifikata ovde, kako bi vam ponudio izbor. Same sertifikate nikada ne dobija, i i dalje ne može ništa da potpiše bez vašeg odobrenja. | Hint under it. |
| `settings.stamp_settings` | Vidljivi pečat... | The button that opens the stamp window in its Settings role — group 4. |
| `settings.signature_level_label` | Nivo potpisa | Legend of the level fieldset. |
| `settings.level_bb` | B-B (samo potpis, bez vremenskog žiga) | Its three radios, B-B first. |
| `settings.level_bt` | B-T (potpis + vremenski žig) | Its three radios, B-B first. |
| `settings.level_blt` | B-LT (potpis + vremenski žig + dokazi o opozivu) | Its three radios, B-B first. |
| `settings.pairings_label` | Povezane aplikacije | Heading of the connected-applications section. |
| `settings.pairings_empty` | Nijedna aplikacija nije povezana. Aplikacija se povezuje tako što zatraži, a vi joj pročitate kod koji Liro Bridge prikaže. | Shown there when nothing is connected. |
| `settings.pairings_when` | Povezana %s, poslednji zahtev %s | On each row: `%s` when it was paired, `%s` when it last asked. |
| `settings.pairings_when_never` | Povezana %s, još nije ništa zatražila | The same row for an application that has never asked. |
| `settings.pairings_revoke` | Prekini vezu | The button on each row. |
| `settings.pairings_revoked` | Veza je prekinuta. Ta aplikacija više ne može da traži potpisivanje na ovom računaru. | Status line after it is pressed. |
| `settings.export_audit_log` | Izvezi dnevnik revizije | The export button. |
| `settings.export_choose_folder` | Izaberite fasciklu za izvoz dnevnika revizije | Title bar of the folder chooser it opens. |
| `settings.export_done` | Dnevnik revizije je izvezen u %s | Status line after the export. `%s` is the folder. |
| `settings.export_failed` | Dnevnik revizije nije mogao biti izvezen. | Status line when the export did not happen. |
| `settings.export_entries_one` | 1 zapis | First part of the summary line under that status. |
| `settings.export_entries` | %d zapisa | First part of the summary line under that status. |
| `settings.export_chains_few` | %d lanca, %s | Second part. Two forms because Serbian needs `lanca` for 2–4 and `lanaca` otherwise — the only place in the catalogue where that distinction is made. |
| `settings.export_chains_many` | %d lanaca, %s | Second part. Two forms because Serbian needs `lanca` for 2–4 and `lanaca` otherwise — the only place in the catalogue where that distinction is made. |
| `settings.export_chains_all_intact` | svaki ispravan | The `%s` inside those two. |
| `settings.export_chains_not_all_intact` | nisu svi ispravni | The `%s` inside those two. |
| `settings.export_check_ok` | provera ispravnosti: u redu | Third part of the summary line. |
| `settings.export_check_broken` | provera ispravnosti: NIJE PROŠLA — dnevnik je izmenjen kod zapisa %d | Third part when the hash chain does not verify. `%d` is a one-based entry number. |
| `settings.export_check_incomplete` | provera ispravnosti: dnevnik nije mogao da se pročita do kraja | Third part when the log could not be read to its end. |
| `settings.export_breaks` | prekidi: %s | Appended when the log has breaks in it. |
| `settings.export_break_at` | prekid %s (%s) | One break inside that list: `%s` file, `%s` reason. |
| `settings.export_break_unguarded` | log nije mogao da se zaključa | The reason in that line. **Also** used by the audit log window for the same four reasons. |
| `settings.export_break_unparseable` | zapis nije mogao da se pročita | The reason in that line. **Also** used by the audit log window for the same four reasons. |
| `settings.export_break_unreachable` | fajl nije mogao da se pročita | The reason in that line. **Also** used by the audit log window for the same four reasons. |
| `settings.export_break_unsound` | poslednji zapis u lancu nije bio ispravan | The reason in that line. **Also** used by the audit log window for the same four reasons. |
| `settings.check_updates_daily` | Automatski proveravaj ažuriranja | Checkbox. |
| `settings.check_updates_now` | Proveri ažuriranja sada | The button beside it. |
| `settings.update_checking` | Provera ažuriranja… | Status line while the check runs. |
| `settings.update_current` | Imate najnoviju verziju (%s). | Status line, positive, when this is the newest release. |
| `settings.update_available` | Dostupna je verzija %s. | Status line when a newer release exists. The window in group 13 opens separately. |
| `settings.update_check_failed` | Provera ažuriranja nije uspela. Proverite vezu sa internetom. | Status line when the check could not reach GitHub. |
| `settings.update_dev_build` | Ovo je razvojna verzija, pa se izdanja ne mogu porediti sa njom. Najnovije izdanje je %s. | Status line on a build with no version to compare. |
| `settings.update_unverified` | Izdanje nije potpisano ključem koji ova verzija poznaje. Ništa nije preuzeto. | Status line, negative, when the release was not signed by a known key. |
| `settings.version_label` | Verzija | Label of the version row. |
| `settings.copy` | Kopiraj | The button on that row. |
| `settings.close` | Zatvori | Footer, the secondary button. **Also** the only button in the Certificates window and the right-hand button in the audit log window. |
| `settings.save` | Sačuvaj | Footer, the primary button. |

## 10. Certificates window

`certificates.html`, opened from the tray.

| Key | Current text | Where it appears |
|---|---|---|
| `certswindow.title` | Sertifikati | Heading and title bar. |
| `certswindow.empty` | Nema pronađenih sertifikata. | Shown when nothing was found. |
| `certswindow.qualified_badge` | kvalifikovan | Badge on a qualified certificate's row. The certificate step does not show this badge, only the test-key one. |

## 11. Audit log window

`auditlog.html`, opened from the tray.

| Key | Current text | Where it appears |
|---|---|---|
| `auditwindow.title` | Dnevnik revizije | Heading and title bar. |
| `auditwindow.empty` | Još uvek nema unosa u dnevniku revizije. | Shown when the log has no entries. |
| `auditwindow.document_count` | %d dokument(a) | On each entry's row. |
| `auditwindow.outcome_approved` | potpisano | The outcome on each entry's row. |
| `auditwindow.outcome_denied` | otkazano | The outcome on each entry's row. |
| `auditwindow.outcome_failed` | neuspešno | The outcome on each entry's row. |
| `auditwindow.outcome_partial` | delimično potpisano | The outcome on each entry's row. |
| `auditwindow.level_bb` | B-B — bez vremenskog žiga | On an entry recorded at B-B. |
| `auditwindow.chain_break` | Ovde počinje novi lanac: %s nije mogao da se nastavi (%s). | A divider row where a new chain begins. `%s` previous file, `%s` one of the four `settings.export_break_*` reasons. |
| `auditwindow.export` | Izvezi | Footer, writes the same two files Settings' own export writes. |

## 12. Pairing window

`pairing.html`. Opened when an application asks to connect.

| Key | Current text | Where it appears |
|---|---|---|
| `pairing.window_title` | Zahtev za povezivanje | Title bar. |
| `pairing.request_label` | Zahtev | Label above the application's declared name. |
| `pairing.origin_label` | Poreklo | Label above its declared origin. |
| `pairing.code_label` | Kod | Label above the six digits. |
| `pairing.code_explain` | Unesite ovaj broj u aplikaciju. | First line of the note under the code. |
| `pairing.code_validity` | Ovaj kod važi 5 minuta. | Second line. The five minutes is `api.PairingCodeTTL` and is correct. |
| `pairing.deny` | Odbij | The only button on this screen. There is no Allow — reading the code out is the approval. |
| `pairing.connected_title` | Povezano | Heading of the screen after the code was accepted. |
| `pairing.close` | Zatvori | The button on that screen. |

## 13. Update prompt

`update.html`. Opened by the daily check, or by **Proveri ažuriranja sada** in Settings.

| Key | Current text | Where it appears |
|---|---|---|
| `update.window_title` | Nova verzija | Title bar, all three states. |
| `update.available_title` | Dostupna je nova verzija | Heading of the offer. |
| `update.version_line` | Imate %s. Nova verzija je %s. | Under it: `%s` what you have, `%s` what is new. |
| `update.released_line` | Objavljena %s. | Under that, when the manifest carries a date. |
| `update.what_happens` | Liro Bridge će preuzeti instalaciju, proveriti njen potpis i pokrenuti je. Program će se pri tome zatvoriti. Dnevnik revizije, podešavanja i povezane aplikacije ostaju. | The paragraph under those two lines. |
| `update.later` | Ne sada | Quiet button. |
| `update.install` | Instaliraj sada | Primary button. |
| `update.working` | Preuzimanje i provera potpisa… | Heading while the installer is fetched and checked. No buttons on that screen. |
| `update.failed_title` | Ažuriranje nije uspelo | Heading when it did not work. |
| `update.failed_body` | Nova verzija nije instalirana i ništa na računaru nije izmenjeno. Možete je preuzeti ručno sa stranice izdanja:<br>%s | The paragraph under it. `%s` is the releases page address, as text — the window never opens a browser. |
| `update.close` | Zatvori | The button on that screen. |

## 14. Tray menu, the Explorer verb, and install-time notices

Native Windows menus and message boxes — no HTML page, so length is constrained by the shell, not by CSS.

| Key | Current text | Where it appears |
|---|---|---|
| `tray.open_window` | Otvori Liro Bridge | First entry of the tray menu, and the double-click action. |
| `tray.settings` | Podešavanja | Second entry. |
| `tray.certificates` | Sertifikati | Third entry. |
| `tray.audit_log` | Prikaži dnevnik revizije | Fourth entry. |
| `tray.quit` | Izađi | Fifth entry. |
| `tray.open` | Otvori | **Unused.** The menu's first entry uses `tray.open_window`. Nothing reads this key. |
| `settings.explorer_menu_verb` | Potpiši koristeći Liro Bridge | The right-click entry itself, on PDF files in Explorer. Registered under `HKCU\SOFTWARE\Classes\SystemFileAssociations\.pdf\shell`. |
| `settings.explorer_menu_verb_short` | Potpiši - Liro Bridge | **Unused.** A shorter verb that already exists in all three catalogues and is wired to nothing. |
| `app.name` | Liro Bridge | **Unused.** Nothing reads this key; every window that needs the product name uses `main.title`. |
| `runtime.missing_title` | Liro Bridge ne može da otvori prozor | A native message box at startup when WebView2 is not installed — before any window can be drawn, so this is the one string that cannot be shown in a page. |
| `runtime.missing_body` | Liro Bridge-u je potreban Microsoft Edge WebView2 Runtime, a on nije instaliran na ovom računaru.<br><br>Preuzmite ga sa:<br>https://go.microsoft.com/fwlink/p/?LinkId=2124703<br><br>Zatim ponovo pokrenite Liro Bridge. | A native message box at startup when WebView2 is not installed — before any window can be drawn, so this is the one string that cannot be shown in a page. |
| `uninstall.notice_title` | Liro Bridge je uklonjen | A native message box shown by the uninstaller. `%s` is the folder that was left behind. |
| `uninstall.notice_body` | Dnevnik revizije nije obrisan. On je zapis o tome šta ste potpisali i ostaje na računaru i posle uklanjanja programa.<br><br>Ostavljeni su ovde:<br>%s<br><br>• audit\\ — dnevnik revizije<br>• config.json — vaša podešavanja<br>• pairings.json i secrets — aplikacije koje ste povezali<br><br>Ovaj folder možete obrisati ručno ako vam više ne treba. | A native message box shown by the uninstaller. `%s` is the folder that was left behind. |

## 15. Error messages

`error.*` keys are resolved from an `errs.Code` by `i18n.CodeKey`, so none of them is referenced by name anywhere in the source. Each is returned to the calling application over the protocol **and**, where noted, shown in the UI.

| Key | Current text | Where it appears |
|---|---|---|
| `error.card_not_present` | Ubacite karticu u čitač. | Protocol error. **Also** the greyed-out reason on a certificate row, in both the certificate step and the Certificates window. |
| `error.cert_expired` | Ovaj sertifikat je istekao. | Protocol error. **Also** a greyed-out certificate row's reason. |
| `error.cert_not_usable` | Ovaj sertifikat se ne može koristiti za potpisivanje. | Protocol error. **Also** a greyed-out certificate row's reason. |
| `error.no_reader` | Čitač kartica nije pronađen. Priključite čitač i pokušajte ponovo. | Protocol error. **Also** reaches the failed screen. |
| `error.smart_card_service_down` | Windows servis za pametne kartice nije pokrenut. | Protocol error. **Also** reaches the failed screen. |
| `error.cert_not_found` | Nije pronađen sertifikat sa tim otiskom. | Protocol error: a thumbprint the caller named is not on this machine. |
| `error.cert_revoked` | Ovaj sertifikat je opozvan. | Protocol error. |
| `error.pin_required` | Kartica traži svoj PIN. | Protocol error. |
| `error.pin_incorrect` | PIN nije tačan. Broj preostalih pokušaja pre blokiranja kartice je ograničen. | Protocol error. |
| `error.pin_locked` | Kartica je blokirana. Otključajte je PUK kodom pomoću softvera izdavaoca. | Protocol error. The report screen says the same thing in its own words as `main.aborted_pin`. |
| `error.sign_failed` | Kartica nije uspela da napravi potpis. | Protocol error. **Also** reaches the failed screen. |
| `error.consent_denied` | Operacija je otkazana. | Protocol error: the person pressed Cancel. |
| `error.consent_timeout` | Odgovor nije stigao na vreme, pa ništa nije potpisano. | Protocol error: nobody answered before the countdown ran out. |
| `error.input_unreadable` | Dokument nije mogao da se pročita. Možda je otvoren u drugom programu ili više nije na istom mestu. | Protocol error. **Also** a row in the report's failure list. |
| `error.pdf_invalid` | Ulaz nije čitljiv PDF dokument. | Protocol error. **Also** a row in the report's failure list. |
| `error.pdf_encrypted` | Ulaz je PDF zaštićen lozinkom. | Protocol error. **Also** a row in the report's failure list. |
| `error.output_exists` | Fajl sa tim imenom već postoji, pa ništa nije prepisano. | Protocol error. The window asks instead — see `consent.output_exists_*` in group 6. |
| `error.output_in_use` | Potpisani dokument nije mogao da zameni postojeći fajl jer je taj fajl otvoren u drugom programu. Zatvorite ga i pokušajte ponovo. | Protocol error. **Also** a row in the report's failure list. |
| `error.output_write_failed` | Potpisani dokument nije mogao biti upisan na disk. Proverite fasciklu i slobodan prostor. | Protocol error. **Also** a row in the report's failure list. |
| `error.stamp_glyph_missing` | Pečat sadrži znak koji font ne podržava: %s (%s). | Protocol error. `%s` the character, `%s` its name. |
| `error.tsa_unavailable` | Servis za vremenske žigove ne odgovara. | Protocol error. The window offers the B-B choice instead — see group 6. |
| `error.tsa_rejected` | Servis za vremenske žigove je odbio zahtev. | Protocol error. |
| `error.tsa_client_cert_invalid` | Klijentski sertifikat za vremenski žig nije mogao biti otvoren. Proverite njegovu lozinku u podešavanjima. | Protocol error. |
| `error.tsa_client_cert_unreadable` | Fajl klijentskog sertifikata za vremenski žig nije mogao biti pročitan. Proverite putanju u podešavanjima. | Protocol error. |
| `error.not_paired` | Ova aplikacija nije uparena sa Liro Bridge-om. | Protocol error. |
| `error.auth_failed` | Zahtev nije mogao biti autentifikovan. | Protocol error. |
| `error.pairing_denied` | Povezivanje je odbijeno. | Protocol error. |
| `error.pairing_expired` | Taj zahtev za povezivanje više ne važi. Pokrenite novi. | Protocol error. |
| `error.pairing_code_incorrect` | To nije kôd prikazan u prozoru. | Protocol error. |
| `error.pairing_in_progress` | Druga aplikacija se upravo povezuje. Pokušajte ponovo za trenutak. | Protocol error. |
| `error.pairing_origin_mismatch` | Povezivanje je potvrđeno sa drugog porekla nego što je zatraženo. | Protocol error. |
| `error.rate_limited` | Aplikacija je poslala previše zahteva. Pokušajte ponovo za trenutak. | Protocol error. |
| `error.request_invalid` | Aplikacija je poslala zahtev koji Liro Bridge ne može da pročita. | Protocol error. |
| `error.version_too_old` | Ova verzija Liro Bridge-a je previše stara za aplikaciju koja ju je pozvala. | Protocol error. |
| `error.job_in_progress` | Ova aplikacija već ima posao potpisivanja u toku. Sačekajte da se završi. | Protocol error. |
| `error.job_not_found` | Taj posao potpisivanja više nije dostupan. Njegov rezultat je već preuzet ili je istekao. | Protocol error. |
| `error.document_signing_disabled` | Potpisivanje celih dokumenata je isključeno u ovoj kopiji Liro Bridge-a. Uključite ga u Podešavanjima. | Protocol error, when the Settings switch is off. |
| `error.certificate_listing_disabled` | Prikazivanje sertifikata drugim programima je isključeno u ovoj kopiji Liro Bridge-a. Uključite ga u Podešavanjima. | Protocol error, when the Settings switch is off. |
| `error.endpoint_not_found` | Ova verzija Liro Bridge-a nema ono što je aplikacija zatražila. Ažurirajte Liro Bridge. | Protocol error: the agent has no such endpoint, which in practice means it is older than the SDK calling it. Added by D-264; a person never sees it in a window. |
| `error.internal` | Došlo je do neočekivane greške. | Protocol error, everything unclassified. |

## 16. Command-line output

Not a window: text written to the console by `liro-bridge certificates`, `sign`, `sign-no-consent` and `sign-digest`. Included because it is text a user reads, but it can be reviewed separately from the windows above.

| Key | Current text | Where it appears |
|---|---|---|
| `sign.stamp_label` | Elektronski potpisano | **Not console output.** This is the wording drawn into the PDF itself, on every visible stamp this program has ever made. Changing it changes new documents only. |
| `certs.readers_none` | Čitači: nijedan nije priključen | `certificates`: the first line. |
| `certs.readers_heading` | Čitači: %d | `certificates`: the first line. |
| `certs.certificates_heading` | Sertifikati: %d | `certificates`: the heading above the list. |
| `certs.card_absent` | nema kartice | `certificates`: each reader's state. |
| `certs.card_present` | kartica prisutna | `certificates`: each reader's state. |
| `certs.usable` | upotrebljiv | `certificates`: each certificate's state. |
| `certs.not_usable` | neupotrebljiv | `certificates`: each certificate's state. |
| `certs.purpose_label` | Namena | `certificates`: the Purpose field. |
| `certs.purpose_signing` | potpisivanje | `certificates`: the Purpose field. |
| `certs.purpose_authentication` | autentifikacija | `certificates`: the Purpose field. |
| `certs.purpose_unknown` | nepoznato | `certificates`: the Purpose field. |
| `certs.issuer_label` | Izdavalac | `certificates`: the Issuer field. Matches Windows' own Serbian certificate UI. |
| `certs.qualified_label` | Kvalifikovan | `certificates`: the Qualified field. |
| `certs.qualified_yes` | da | `certificates`: the Qualified field. |
| `certs.qualified_yes_qscd` | da — kvalifikovani sertifikat na QSCD uređaju | `certificates`: the Qualified field. |
| `certs.qualified_no` | ne | `certificates`: the Qualified field. |
| `certs.qualified_unknown` | nepoznato — lista poverenja nije dostupna | `certificates`: the Qualified field. |
| `certs.valid_label` | Važi | `certificates`: the Valid field. |
| `certs.valid_range` | %s do %s | `certificates`: the Valid field. |
| `certs.thumbprint_label` | Otisak | `certificates`: the Thumbprint field. Matches Windows' own Serbian certificate UI. |
| `certs.storage_label` | Smeštaj | `certificates`: the Storage field. |
| `certs.storage_smart_card` | pametna kartica | `certificates`: the Storage field. |
| `certs.storage_software` | softver | `certificates`: the Storage field. |
| `certs.reason_label` | Razlog | `certificates`: the Reason field on an unusable certificate. Says the same three things as `error.cert_expired`, `error.card_not_present` and `error.cert_not_usable` in different words — see finding F10. |
| `certs.reason_expired` | sertifikatu je istekao rok važenja | `certificates`: the Reason field on an unusable certificate. Says the same three things as `error.cert_expired`, `error.card_not_present` and `error.cert_not_usable` in different words — see finding F10. |
| `certs.reason_card_not_present` | kartica nije prisutna | `certificates`: the Reason field on an unusable certificate. Says the same three things as `error.cert_expired`, `error.card_not_present` and `error.cert_not_usable` in different words — see finding F10. |
| `certs.reason_not_usable` | sertifikat se ne može koristiti za potpisivanje | `certificates`: the Reason field on an unusable certificate. Says the same three things as `error.cert_expired`, `error.card_not_present` and `error.cert_not_usable` in different words — see finding F10. |
| `certs.tsl_heading` | Lista poverenja: sekvenca %d, izdata %s (stara %d dana, %s) | `certificates`: the Trusted List line. |
| `certs.tsl_source_embedded` | ugrađena | The `%s` at the end of that line. |
| `certs.tsl_source_cache` | iz keša | The `%s` at the end of that line. |
| `certs.tsl_source_network` | upravo osvežena | The `%s` at the end of that line. |
| `certs.tsl_stale_warning` | upozorenje: lista poverenja nije uspešno osvežena sa mreže u poslednjih 30 dana | Under it, when no successful fetch has happened in 30 days — **or** when none has ever happened. See finding F7. |
| `sign.in_required` | --in je obavezan. | Argument errors, `sign` and `sign-no-consent`. |
| `sign.thumbprint_required` | --thumbprint je obavezan. | Argument errors, `sign` and `sign-no-consent`. |
| `sign.no_input_files` | nijedan ulazni fajl ne odgovara --in. | Argument errors, `sign` and `sign-no-consent`. |
| `sign.invalid_digest` | --digest mora imati tačno 64 heksadecimalna znaka (SHA-256 heš). | Argument error, `sign-digest`. |
| `sign.level_invalid` | --level mora biti b-t ili b-lt. | Argument errors, `sign-no-consent`. |
| `sign.on_tsa_failure_invalid` | --on-tsa-failure mora biti abort ili b-b. | Argument errors, `sign-no-consent`. |
| `sign.stamp_position_invalid` | --stamp-position mora biti bottom-right, bottom-left, top-right ili top-left. | Argument errors, `sign-no-consent`. |
| `sign.stamp_position_xy_conflict` | --stamp-position i --stamp-xy se međusobno isključuju. | Argument errors, `sign-no-consent`. |
| `sign.stamp_xy_invalid` | --stamp-xy mora biti "x,y" u tačkama. | Argument errors, `sign-no-consent`. |
| `sign.output_exists` | izlazni fajl već postoji; koristite --force za prepisivanje. | Per-document errors, `sign-no-consent`. |
| `sign.output_is_another_input` | njegov potpis bi zamenio drugi dokument iz ove grupe | Per-document errors, `sign-no-consent`. |
| `sign.already_signed_nothing_left` | Svi ulazi su već potpisani dokumenti (imena se završavaju na %s). Ništa nije potpisano. Koristite --resign da ipak budu potpisani. | `sign-no-consent`: what `--resign` did or would do. |
| `sign.already_signed_skipped_one` | 1 ulaz je već potpisan dokument (ime se završava na %s) i preskočen je. Koristite --resign da bude potpisan. | `sign-no-consent`: what `--resign` did or would do. |
| `sign.already_signed_skipped_many` | %d ulaza su već potpisani dokumenti (imena se završavaju na %s) i preskočeni su. Koristite --resign da budu potpisani. | `sign-no-consent`: what `--resign` did or would do. |
| `sign.already_signed_resigning_one` | 1 ulaz je već potpisan dokument (ime se završava na %s); zadat je --resign, pa se potpisuje ponovo. | `sign-no-consent`: what `--resign` did or would do. |
| `sign.already_signed_resigning_many` | %d ulaza su već potpisani dokumenti (imena se završavaju na %s); zadat je --resign, pa se potpisuju ponovo. | `sign-no-consent`: what `--resign` did or would do. |
| `sign.signed_label` | Potpisano: | `sign-no-consent`: the per-document lines. |
| `sign.certificate_label` | Sertifikat: | `sign-no-consent`: the per-document lines. |
| `sign.level_label` | Nivo: | `sign-no-consent`: the per-document lines. |
| `sign.batch_summary` | Potpisano %d/%d dokumenata | `sign-no-consent`: the last line. |
| `sign.revocation_too_large` | Podaci o opozivu su bili preveliki za ugrađivanje (%s); sačuvano na nivou B-T. | Appended to the level when B-LT had to fall back to B-T. |
| `sign.clock_drift_warning` | Upozorenje: sat vašeg računara (%s) i pouzdani vremenski žig (%s) razlikuju se za više od pet minuta. Proverite sat na računaru. | Printed when the machine clock and the trusted timestamp disagree by more than five minutes. The five minutes is `pades.clockDriftWarnThreshold` and is correct. |
| `sign.test_key_warning` | TEST POTPIS — napravljen softverskim tokenom, ne pravom karticom. | Printed when the soft token made the signature. |
| `sign.report_heading` | Potpisano %d/%d | `sign-digest`: the heading of its timing report. |
| `sign.first_signature_label` | Prvi potpis: | `sign-digest`: the timing rows. |
| `sign.median_subsequent_label` | Medijana narednih: | `sign-digest`: the timing rows. |
| `sign.total_label` | Ukupno: | `sign-digest`: the timing rows. |
| `sign.pin_policy_label` | PIN politika: | `sign-digest`: the PIN-policy row. |
| `sign.pin_policy_per_batch` | jedan PIN otključava celu seriju | `sign-digest`: the PIN-policy row. |
| `sign.pin_policy_per_signature` | kartica traži PIN za svaki potpis | `sign-digest`: the PIN-policy row. |
| `sign.pin_policy_unknown` | još nije utvrđeno | `sign-digest`: the PIN-policy row. |
| `sign.timestamp_label` | Vremenski žig: | **Unused.** `levelLine` no longer prints a timestamp row; nothing reads either key. |
| `sign.no_timestamp` | nema | **Unused.** `levelLine` no longer prints a timestamp row; nothing reads either key. |

---

## Findings

Things noticed while reading all of it. Nothing here has been changed.
Each one names the keys, so it can be accepted or rejected on its own.

### The catalogues are in step with each other

This is the one thing that came back clean, and it is worth recording
before the problems:

- All three files hold **exactly the same 364 keys**. Nothing is present
  in one and missing from another, in either direction.
- Every `%s`, `%d` and `%.0f` matches across the three, in count and in
  order. No string can crash or mis-render on a locale switch.
- Embedded line breaks match across the three.
- `sr-Cyrl.json` is a **character-exact transliteration** of
  `sr-Latn.json` — all 364 strings, checked by transliterating the
  Cyrillic back to Latin and comparing. The two Serbian catalogues have
  not drifted apart in wording at any point.

The practical consequence for this pass: once the Latin wording is
settled, `sr-Cyrl.json` can be produced mechanically from it, and only
`en.json` needs a human. Four strings are identical in all three by
design and should stay that way — `app.name`, `main.title`,
`settings.tsa_preset_freetsa` and `settings.tsa_url_label`.

### Text that does not match what the program does

**F1. Six keys are dead — nothing in the program reads them.**

| Key | Current text | Why it is dead |
|---|---|---|
| `app.name` | Liro Bridge | Nothing reads it. Every window that needs the name uses `main.title`. |
| `consent.window_title` | Odobrite potpis | The certificate screen became a *step* of the signing window rather than a window of its own. `ui.NewWindow` is called once, with `main.title`, and nothing ever calls `SetTitle`. Referenced only by three test files, which is why it survived. |
| `tray.open` | Otvori | The tray's first entry is built from `tray.open_window`. |
| `settings.explorer_menu_verb_short` | Potpiši - Liro Bridge | Wired to nothing. Relevant to what you want below. |
| `sign.timestamp_label` | Vremenski žig: | `levelLine` no longer prints a timestamp row. |
| `sign.no_timestamp` | nema | Same. |

Worth deciding deliberately: `consent.window_title` says "Odobrite
potpis", which is a better title for the approval screen than "Liro
Bridge". The key being dead may be the bug, not the string.

**F2. `place.title` is used as the title of a file chooser.**

`stampwindow_windows.go:284` opens the Windows file picker with
`c.T("place.title")` — so when you press **Postavi na stranu…** in
Settings, a file-open dialog appears titled **"Pozicija potpisa"**. It
names the window that comes *after* the dialog, not the dialog. The
right string already exists: `main.choose_files` ("Izaberite PDF
dokumente"), which is what the signing window's own chooser uses.

**F3. The Trusted List warning claims a refresh that may never have been attempted.**

`certs.tsl_stale_warning` — "lista poverenja nije uspešno osvežena sa
mreže u poslednjih 30 dana". `tslNeedsStaleWarning` returns true in two
different situations: the last successful fetch is over 30 days old
(`staleWarningAge`, which the string describes correctly), **and** the
list is still the embedded seed, meaning no fetch has ever succeeded. In
the second case "in the last 30 days" is technically true but points at
the wrong problem — a fresh install with no network has never refreshed
at all, and the number invites the reader to wait it out.

**F4. Serbian's 2–4 plural is handled in exactly one place.**

The code knows the rule and applies it — `chainCountKey` in
`tray_windows.go:548` picks `settings.export_chains_few` ("lanca") for
2, 3, 4 and `..._many` ("lanaca") otherwise, with the 12–14 exception,
and the comment explains why. Nothing else counting anything does this:

| Key | Current text | What a small number produces |
|---|---|---|
| `main.document_count` | %d dokumenata, %s | "2 dokumenata, 1,4 MB" — should be *dokumenta* |
| `main.notice_folder_scanned` | %s: dodato %d PDF dokumenata. | "dodato 2 PDF dokumenata" — should be *dodata 2 PDF dokumenta*; and for one file it reads "dodato 1 PDF dokumenata", with no `_one` form to fall back to |
| `main.report_output_collision_many` | %d dokumenata nije potpisano: … | "2 dokumenata nije potpisano" |
| `auditwindow.document_count` | %d dokument(a) | dodges it with "(a)" |
| `consent.document_count` | %d dokument(a) za potpisivanje | dodges it the same way |
| `main.report_already_signed_many` | Preskočeno dokumenata jer su već potpisani: %d. | dodges it by moving the number to the end, which is why this line reads unlike every other line on that screen — compare the English, "%d documents were skipped because they are already signed." |

So the screen currently uses three different strategies for one problem:
get it right (chains), print "(a)", or rewrite the sentence around it.
Some of the others are right by accident — "%d zapisa" and "%d ulaza"
work for every number because the genitive singular and plural coincide.

**F5. Two errors name a Settings field that Settings calls something else.**

- Settings labels the field **"Klijentski sertifikat za servis za
  vremenske žigove (PKCS#12)"** (`settings.tsa_client_cert_label`).
- `error.tsa_client_cert_invalid` says "Klijentski sertifikat **za
  vremenski žig** … Proverite njegovu lozinku u podešavanjima."
- `error.tsa_client_cert_unreadable` says the same thing the same way.

Both errors send the reader to Settings to find a field under a name
Settings does not use.

### Wording that differs between two places meaning the same thing

**F6. "Otkaži" and "Odustani" are both used for Cancel — and on Windows they are different words.**

| Key | Current text | Where |
|---|---|---|
| `consent.cancel` | Otkaži | Certificate step, and all three post-approval questions |
| `place.cancel` | Odustani | Placement picker |
| `stampwindow.cancel` | Odustani | Signing-method screen, both roles |

All three are "Cancel" in `en.json`. I read the strings out of this
machine's Serbian Latin Windows resources rather than going from memory
— `C:\Windows\System32\sr-Latn-RS`:

- `user32.dll.mui` string 801 = **Otkaži** — the Cancel button.
- `user32.dll.mui` string 802 = **&Odustani** — the *Abort* button, from
  the Abort/Retry/Ignore box.
- `comdlg32.dll.mui` string 372 = **Otkaži** — the Cancel button in
  every file and folder dialog the program opens.

So on Serbian Windows "Odustani" means *abort*, and two of this
program's screens use it where the platform says "Otkaži". The
placement picker is the sharper case: its Cancel sits a few pixels from
a file dialog that says "Otkaži".

**F7. "fajl" and "datoteka" are both used for *file*. 13 keys to 2.**

*fajl*: `audit.chain_continued`, `audit.chain_continued_here`,
`consent.files_label`, `consent.output_exists_path_label`,
`consent.output_exists_title`, `error.output_exists`,
`error.output_in_use`, `error.tsa_client_cert_unreadable`,
`settings.export_break_unreachable`, `settings.output_suffix_label`,
`settings.tsa_client_cert_placeholder`, `sign.no_input_files`,
`sign.output_exists`.

*datoteka*: `main.file_filter_all` ("Sve datoteke"),
`settings.explorer_menu` ("…na PDF datotekama").

Windows uses **datoteka** without exception — "Sve datoteke",
"Kopiraj datoteku", "Preimenuj datoteku", "Izbriši datoteku" (shell32
strings 16876–16878). Note the awkward consequence today:
`main.file_filter_all` says "Sve datoteke" inside a dialog opened by a
program that calls the same things fajlovi everywhere else, because that
one string was matched to the platform and the rest were not.

**F8. "folder" and "fascikla" are both used for *folder*. 5 keys to 2.**

*folder*: `main.empty_hint`, `main.open_output`,
`main.output_beside_input`, `main.report_output_various`,
`uninstall.notice_body`.

*fascikla*: `error.output_write_failed`, `settings.export_choose_folder`.

Windows uses **fascikla** throughout ("Kreirajte novu fasciklu",
"Kopiraj u fasciklu…", "Otvori lokaciju fascikle"). Same shape of
problem as F7, and the same two candidates: match the platform, or be
consistently colloquial. What it cannot stay is split.

**F9. The timestamp authority has three names.**

| Name | Keys |
|---|---|
| "servis za vremenske žigove" | `settings.tsa_label`, `settings.tsa_user_label`, `settings.tsa_password_label`, `settings.tsa_client_cert_label`, `settings.tsa_client_cert_password_label`, `error.tsa_rejected`, `error.tsa_unavailable` |
| "izdavalac vremenskog žiga" | `consent.tsa_configure`, `consent.tsa_reason_not_configured`, `consent.tsa_reason_unreachable` |
| "za vremenski žig" | `error.tsa_client_cert_invalid`, `error.tsa_client_cert_unreadable` |

The split is not random, which makes it worse: Settings says *servis*
and the screen that sends you to Settings says *izdavalac*. "Podesi
izdavaoca vremenskog žiga" is a button whose whole job is to open a
fieldset headed "Servis za vremenske žigove".

There is a second reason to drop *izdavalac* for this: the catalogue
already uses that word for two other things — `certs.issuer_label`
("Izdavalac", the certificate's issuer) and `error.pin_locked`
("softvera izdavaoca", the card's issuer). Three issuers on one screen
is one too many.

English has a milder version of the same split — "timestamp authority"
in `consent.tsa_*`, "timestamp service" in `error.tsa_rejected` and
`error.tsa_unavailable` — and additionally uses the bare abbreviation
**TSA** in four Settings labels (`settings.tsa_user_label`,
`settings.tsa_password_label`, `settings.tsa_client_cert_label`,
`settings.tsa_client_cert_password_label`) where Serbian spells the
whole thing out. Whatever you settle on in Serbian, English needs the
same decision made once.

**F10. "Where the signed documents go" is said four ways.**

| Key | Current text |
|---|---|
| `main.output_beside` | Pored svakog dokumenta |
| `main.output_beside_input` | isti folder u kome je dokument |
| `main.report_output_various` | folder svakog dokumenta |
| `settings.output_folder_default` | Ostavite prazno da se čuva pored svakog dokumenta. |

The first two are on the *same row of the same screen* — the button and
the value it sets. A person reads "isti folder u kome je dokument" and
presses a button labelled "Pored svakog dokumenta" to get it.

**F11. The same field is labelled two ways between the main window and Settings.**

`main.output_label` = "Sačuvaj potpisane u" against
`settings.output_folder_label` = "Sačuvaj potpisane dokumente u". Same
setting, same words minus a noun. The report's own label is a third
shape, `main.report_output_label` = "Sačuvano u".

Likewise `main.report_level_label` = "Nivo potpisa" and
`settings.signature_level_label` = "Nivo potpisa" agree — but B-B itself
is described three different ways depending on where you meet it:
`settings.level_bb` "B-B (samo potpis, bez vremenskog žiga)",
`consent.level_bb` "Nivo B-B — bez vremenskog žiga",
`auditwindow.level_bb` "B-B — bez vremenskog žiga".

**F12. Pairing is "povezivanje" in 16 strings and "uparivanje" in one.**

`error.not_paired` = "Ova aplikacija nije **uparena** sa Liro
Bridge-om." Everything else on the subject — the window title, the
Settings section, the buttons, the other four pairing errors — uses
*povezati/veza*: "Zahtev za povezivanje", "Povezane aplikacije",
"Prekini vezu", "Povezivanje je odbijeno". `en.json` has the identical
split ("not paired" against "Connected applications").

**F13. "Vidljivi pečat…" opens a window headed "Metod potpisivanja".**

`settings.stamp_settings` promises a stamp; `stampwindow.title` delivers
a signing method. The window does both jobs — it is the method step
during signing and the stamp preferences from Settings — but the button
and the heading should agree about which one the reader just asked for.

**F14. "Prionulo uz" + "dole levo" does not compose in Serbian.**

`place.js:161` builds the footer line as `place.snapped_to` + " " +
`place.corner_*`, producing **"Prionulo uz dole levo"**. *Uz* takes the
accusative and cannot govern an adverbial phrase; it needs a noun —
"Prionulo uz donju levu ivicu", or a different construction entirely.
English composes fine, which is why it was not caught: "Snapped to" +
"the bottom left".

Note that the `place.corner_*` strings exist only for this one line —
the corner *buttons* use `stampwindow.position_*` ("Dole levo",
capitalised) — so these four can be rewritten freely without touching
anything else.

**F15. `sign.stamp_label` says "Elektronski potpisano"; English says "Digitally signed".**

This is the one string drawn **into the PDF**, on every visible stamp
the program has ever produced. The Serbian is the eIDAS and Serbian
statutory term; the English drifted to "digitally", which in this field
means something narrower. Flagging it because it is the highest-stakes
string in the catalogue and the only one whose old values stay readable
in documents already signed — a change applies to new documents only.

### Punctuation and typography

**F16. Two kinds of ellipsis, on no consistent rule.**

Three dots (`...`): `main.browse`, `main.output_change`,
`main.export_report`, `settings.stamp_settings`,
`consent.state_preparing_card`, `main.stopping`, `consent.files_overflow`.

One ellipsis character (`…`): `stampwindow.place_button`,
`consent.looking_for_certificates`, `main.sign_opening`,
`settings.update_checking`, `update.working`.

There is almost a rule here — "…" for progress, "..." for a button that
opens something — but it breaks in both directions:
`stampwindow.place_button` is a button that opens something and uses
"…", while `main.stopping` and `consent.state_preparing_card` are
progress and use "...". Both locales are split identically, so this is a
design decision that was never made, not a translation slip. Windows'
own Serbian resources use three dots throughout ("Otvori pomoću...",
"Izračunavanje...", "Otkazivanje...").

**F17. Straight ASCII quotes where Serbian uses „ “.**

`settings.explorer_menu` = `Dodaj "Potpiši koristeći Liro Bridge" u meni
desnog klika na PDF datotekama`. Serbian typographic quotes are „…“, and
Windows' own Serbian resources use them — shell32 string 4196 is
`Preimenuj „%1!ls!“ u „%2!ls!“`.

**F18. "Podešavanja" is capitalised in two errors and lowercased in two.**

Capital: `error.certificate_listing_disabled`,
`error.document_signing_disabled` ("Uključite ga u **P**odešavanjima").
Lowercase: `error.tsa_client_cert_invalid`,
`error.tsa_client_cert_unreadable` ("u **p**odešavanjima"). All four
point at the same window, whose title is "Podešavanja".

### Two smaller notes

- `consent.eta_label` = "Preostaje otprilike %s". Windows says
  **"Preostalo približno %1!s!"** for the same idea (shell32 string
  32922, the file-copy dialog). Not wrong, just not the phrasing people
  have already read a thousand times.
- The program says **sertifikat** throughout; Windows' Serbian
  certificate UI says **certifikat** (`cryptui.dll.mui`). I would leave
  this one alone — *sertifikat* is the form Serbian electronic-signature
  law uses, and it is the term the program's users will have met in that
  context. Recording it only so the divergence is a decision rather
  than an oversight. Two neighbouring terms *do* match Windows exactly
  and should not be touched: `certs.thumbprint_label` "Otisak" and
  `certs.issuer_label` "Izdavalac".

---

## The Explorer verb

You want `settings.explorer_menu_verb` — currently **"Potpiši koristeći
Liro Bridge"**, 29 characters — shorter, and sitting beside its
neighbours rather than shouting over them.

### What the neighbours actually say

Read out of this machine's Serbian Latin Windows resources rather than
recalled — `C:\Windows\System32\sr-Latn-RS\shell32.dll.mui`, by string
ID:

| Verb | ID | Length |
|---|---|---|
| Otvori | 12850, 8496 | 6 |
| Otvori pomoću... | 9016 | 16 |
| Odštampaj | 31250 | 9 |
| Deljenje | 33011, 33016 | 8 |
| Iseci | 31244 | 5 |
| Kopiraj | 31246, 4146 | 7 |
| Nalepi | 31380 | 6 |
| Preimenuj | 31242, 4148 | 9 |
| Izbriši | 31252, 4147 | 7 |
| Svojstva | 31259, 16534 | 8 |
| Kopiraj kao putanju | 30329 | 19 |
| Pošalji u | 30312 | 9 |
| Kreiraj prečice ovde | 29707 | 20 |
| Zakači na traku zadataka | 5386 | 24 |
| Još opcija | 31153 | 10 |

The shape is consistent: **bare imperative, second person singular, one
or two words, no product name, no gerund**. The longest ones are the
ones that have to name a destination ("Zakači na traku zadataka"), and
even those do not name the program doing the work. Nothing in the
platform's own menu is built as *verb + koristeći + product*.

For reference, the entry as it stands is registered on this machine
right now, under
`HKCU\SOFTWARE\Classes\SystemFileAssociations\.pdf\shell\LiroBridgeSign`,
reading "Potpiši koristeći Liro Bridge".

### Candidates

| Candidate | Length | Notes |
|---|---|---|
| **Potpiši** | 7 | Sits exactly with Otvori / Odštampaj / Preimenuj. The program's identity is carried by the icon — `applyExplorerMenu` already extracts the real Liro mark via `ui.IconFilePath()` and registers it with the verb — so the entry is not anonymous. Ambiguous only if a second signing tool is ever installed. |
| **Elektronski potpiši** | 19 | Says which kind of signing without naming the product. Same length as "Kopiraj kao putanju". Still a bare imperative. |
| **Potpiši – Liro Bridge** | 21 | The pattern third-party apps use. `settings.explorer_menu_verb_short` already holds almost exactly this, unused — though with an ASCII hyphen, which should be an en dash if you take it. |
| **Potpiši Liro Bridge-om** | 22 | Instrumental case, which is the idiomatic Serbian for "using X" and matches the house style elsewhere ("sa Liro Bridge-om" in `error.not_paired`). Still names the product, and the hyphenated declension is awkward at a glance. |

My recommendation is **"Potpiši"**, with **"Elektronski potpiši"** if
you want the menu to say what kind of signing without the program's
name. The icon is already doing the identifying work, and "Potpiši" is
the only candidate that a Serbian Windows user would not be able to pick
out as third-party.

### Two things that have to move with it

1. `settings.explorer_menu` **quotes the verb**: `Dodaj "Potpiši
   koristeći Liro Bridge" u meni desnog klika na PDF datotekama`. The
   checkbox and the verb must be changed together or Settings will
   promise an entry that does not exist. (See also F17 on the quote
   characters, and F7 — this is one of the two strings that says
   *datoteka*.)
2. `settings.explorer_menu_verb_short` is dead (F1). Either wire it up,
   set it to whatever you choose here, or delete it from all three
   catalogues — leaving a second, differently-worded verb in the file is
   how the two drift apart later.

---

## Changes requested during review

Recorded as they were asked for, not yet made. Two of the three need an
answer before they can be.

### C1. The origin in Settings, in the same face as everything else

**Asked for:** "The consent screen's origin: put the link on its own
line under the 'Poreklo' label, and render it in the same font as
everything else. It currently sits inline and in a different face."

**Settled (2026-09-13).** It is **Settings → Povezane aplikacije**, the
`<code>` element — the one genuinely in a different typeface. **The
pairing window is to be left alone.** And it is confirmed that the
origin is not a link and is not to become one.

So the change is `settings.css:135`, `.pairing-origin`: drop
`font-family: var(--liro-font-family-mono)` so the origin renders in the
page's own face. It is already on its own line — `settings.js:21-33`
stacks name, origin and "when" as siblings inside `.pairing-text` — so
nothing moves. Keep `overflow-wrap: anywhere`: an origin is one
unbroken token by nature and removing it would widen the window.

Worth deciding at the same time, since it is the same line: the origin
stays `--liro-color-text-secondary` and `--liro-font-size-small`, which
is what distinguishes it from the application name above it once the
typeface no longer does.

<details>
<summary>What was surveyed before this was settled</summary>

The certificate/approval screen shows
**no origin at all** — `consent.html` and `buildConsentInit` carry the
application *name* (`consent.application_label`) and nothing else. The
origin appears in exactly two places, and neither matches the
description exactly:

| Where | Markup | On its own line? | Face |
|---|---|---|---|
| **Pairing window**, under `pairing.origin_label` | `pairing.html:42` — `.field` is `flex-direction: column` | **Yes, already** | Same family, but `--liro-font-size-heading` + semibold, so it reads as a different face from the small regular label above it (`pairing.css:47`) |
| **Settings → Povezane aplikacije**, each row | `settings.js:25` builds a `<code>` element | **Yes, already** | `--liro-font-family-mono` (`settings.css:136`) — genuinely a different typeface |

So "in a different face" was true of both and "sits inline" of neither,
which is what the question above resolved.

It is not a link in either place and cannot become one: no page in this
program navigates anywhere (`update.html`'s own comment gives the
reason), and the origin is caller-supplied text shown verbatim so that a
person can notice `http://` where they expected `https://`.

</details>

### C2. The pairing success screen: a green check and "Uspešno povezano", nothing else

**Asked for:** the screen after pairing succeeds should be a green
circle-check icon and the words "Uspešno povezano". Nothing else.

**What is there now** (`pairing.html`, `#state-connected`):

1. A hand-drawn circle-check `<svg>`, inline, `aria-hidden`. Its comment
   says it is drawn here "because one glyph is not a dependency".
2. `pairing.connected_title` — **"Povezano"**.
3. The application's name, in a scrolling region (`#connected-name`).
4. The **Zatvori** button.

**The text change is unambiguous** and is the one part of this that
belongs to the text pass:

| Key | Now | Asked for |
|---|---|---|
| `pairing.connected_title` | Povezano | **Uspešno povezano** |

with `en.json` and `sr-Cyrl.json` to follow. Everything else in the
request is layout: remove the application name (3), and replace the
hand-drawn mark (1) with the lucide `circle-check-big`.

**Settled (2026-09-13): Zatvori stays.** "Nothing else" meant no
explanatory paragraph, not no way out. So the screen becomes: the icon,
`pairing.connected_title`, and the button — with the application name
(`#connected-name`, and the scrolling region around it) removed.

**Still blocked: the SVG has not arrived.** It was said to be attached
and nothing came through; the owner will paste it in the next session.
The instruction was to use it as the source "rather than drawing your
own", so **nothing has been drawn** — do not substitute one. When it
arrives this is a small edit to `#connected-check` in `pairing.html`.

One thing to settle when it does: **green from where.** The icon should
take its colour from a token (`--liro-color-*`) like every other value
on these pages rather than a hex value inside the SVG. The existing
hand-drawn mark uses `currentColor` for exactly this reason, and
whatever lucide markup arrives will need the same treatment.

### C3. `place.unavailable` describes the wrong reason on the protocol path

Found while watching demo B rather than asked for, and noted here
because it is a text change. `place.unavailable` reads "Ovaj dokument ne
može da se prikaže, pa se pečat postavlja po uglu" — *this document
cannot be displayed*. That is true for a local document the renderer
cannot draw, which is the case it was written for. It is also what a
person sees when a **protocol** batch reaches the same screen, where the
real reason is different: the documents arrived over a socket and are
not files at all, so there is nothing to open, drawable or not
(`signflow_windows.go`, `signAtAChosenPosition`). One string is doing
two jobs and is only right about one of them.

**Settled, and done in the text pass.** The string was split in two,
one per reason, and each now has exactly one producer:

| Key | Producer | The reason it names |
|---|---|---|
| `place.unavailable` | `stampwindow_windows.go` | a document on this machine the renderer cannot draw |
| `place.unavailable_no_file` | `signflow_windows.go`, `signAtAChosenPosition` | a batch from an application, with no file to draw |

Both are reachable — that was measured before the split rather than
assumed, and it is why the answer is two strings rather than a reworded
one. The second was then confirmed on screen: choosing "Potpiši
birajući poziciju potpisa" in demo B, against the soft token, put it
directly under the option, with no picker and no file dialog.

**A correction to this document, recorded because it misled once.** A
reading of demo B reported the program saying the *second* sentence and
concluded that this finding was stale or that two strings already
existed. Neither: the second sentence is new, written by the text pass,
and this document describes the catalogue as it stood before it. The
demo was run against a binary built from the working tree.

**A flow question this leaves open, for its own pass and not for a text
one.** The method screen still offers "Potpiši birajući poziciju
potpisa" on a path where it cannot work. It refuses clearly and at once
— the sentence appears directly under the option, nothing opens, and
the corners are there to choose instead — which is the right way to
refuse. Not offering it at all would be better than refusing it well.
That is a change to what the screen shows rather than to what it says,
so the text pass deliberately left it alone.
