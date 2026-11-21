# Cancellierotron

Il Cancelliere, il bot Telegram del gruppo del Caffè Letterario Bari.

## Funzioni

Tutti gli orari sono riferiti a Europe/Rome.

### Benvenuto
Quando entra un nuovo membro (non bot), il bot lo saluta per nome e mostra un pulsante "ℹ️ Informazioni" che apre la pagina info del gruppo.

### Candidature per il libro del mese
Il libro del mese è il libro che il Caffè Letterario propone per quel mese: non lo leggono tutti, ma viene consigliato a chi vuole, anche a chi non fa parte del gruppo. Agli incontri ognuno continua a portare il libro che sta leggendo.

- **Il 25 di ogni mese, alle 10:00**, il bot apre le candidature con un messaggio e due pulsanti: "Mi candido" e "Ritiro la candidatura". Ci si può candidare fino alle 10:00 del giorno 1.
- Il messaggio si aggiorna con l'elenco puntato dei candidati; i nomi sono cliccabili e aprono il profilo.
- **Il 1° di ogni mese, alle 10:00**, il bot chiude le candidature (toglie i pulsanti) ed estrae a sorte chi sceglie il libro del mese. Se non c'è nessun candidato lo comunica.

### Sondaggi per l'incontro del sabato
**Ogni mercoledì, alle 10:00**, il bot invia due sondaggi non anonimi per l'incontro di sabato alle 18:30:

1. "Sabato GG/MM ci vediamo alle 18:30 [luogo]: come partecipi?"
   - Bar e poi pizza
   - Parco e poi pizza
   - Vengo solo al bar
   - Vengo solo al parco
   - Vengo solo per la pizza
2. "Chi vuole leggere dei passi durante il prossimo incontro del caffè letterario?"
   - Leggo
   - Ascolto

Il bot fissa in alto il primo sondaggio e lo sblocca la domenica a mezzanotte.
Se un admin ha annullato l'incontro di quel sabato, i sondaggi non vengono inviati.

### Promemoria del sabato
**Ogni sabato, alle 10:00**, il bot ricorda l'incontro della sera (con il luogo, se impostato) e mostra in un elenco puntato, con i nomi cliccabili, chi ha detto che partecipa nel primo sondaggio del mercoledì. Se nessuno ha ancora risposto, invita a farlo. Se mercoledì il sondaggio non è partito, ricorda solo l'incontro.
Se l'incontro è stato annullato, il promemoria non parte.

### Comandi (solo admin)
| Comando | Effetto |
|---|---|
| `/annulla` | Annulla il prossimo incontro del sabato: quella settimana non partono né i sondaggi né il promemoria. Durante `/luogo` annulla l'operazione. |
| `/luogo` | Chiede il luogo dell'incontro, con la preposizione davanti (es. "al Vergnano"), massimo 200 caratteri. Il luogo compare nel primo sondaggio e nel promemoria del sabato. |
| `/cancella_luogo` | Rimuove il luogo dell'incontro. |
| `/debug` | Comando nascosto. Mostra dei pulsanti per eseguire subito: apertura candidature, estrazione, sondaggi dell'incontro, promemoria del sabato, benvenuto. |

### Chat private
Il bot funziona solo nei gruppi: in chat privata non risponde.

## Avvio

Mettere il token del bot nel file `TOKEN` (viene incluso nel binario) oppure passarlo con `-t`.

```
go build && ./cancellierotron [-t TOKEN] [-i URL_PAGINA_INFO]
```

I dati di ogni chat (candidati, luogo, incontro annullato, sondaggio fissato e chi partecipa) sono salvati nella cartella di cache dell'utente, sotto `ciambellanotron`.

## Licenza

[WTFPL](LICENSE).
