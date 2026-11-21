# Cancellierotron

Il Cancelliere, il bot Telegram del gruppo del Caffè Letterario Bari.

## Funzioni

Tutti gli orari sono riferiti a Europe/Rome.

### Benvenuto
Quando entra un nuovo membro (non bot), il bot lo saluta per nome e mostra un pulsante "ℹ️ Informazioni" che apre la pagina info del gruppo.

### Candidature per il libro del mese
- **Il 25 di ogni mese, alle 10:00**, il bot apre le candidature con un messaggio e due pulsanti: "Mi candido" e "Ritiro la candidatura".
- Il messaggio si aggiorna con l'elenco dei candidati.
- **Il 1° di ogni mese, alle 10:00**, il bot chiude le candidature (toglie i pulsanti) ed estrae a sorte chi sceglie il libro del mese. Se non c'è nessun candidato lo comunica.

### Sondaggi per l'incontro del sabato
**Ogni mercoledì, alle 10:00**, il bot invia due sondaggi non anonimi per l'incontro di sabato alle 18:30:

1. "Sabato GG/MM ci vediamo alle 18:30 [luogo]: come partecipi?"
   - Bar e poi pizza
   - Parco e poi pizza
   - Vengo solo al bar
   - Vengo solo al parco
   - Vengo solo per la pizza
2. "Chi vuole leggere dei passi a questa sessione di caffè letterario?"
   - Leggo
   - Ascolto

Il bot fissa in alto il primo sondaggio e lo sblocca la domenica a mezzanotte.
Se un admin ha annullato l'incontro di quel sabato, i sondaggi non vengono inviati.

### Comandi (solo admin)
| Comando | Effetto |
|---|---|
| `/annulla` | Annulla il prossimo incontro del sabato: quella settimana i sondaggi non partono. Durante `/luogo` annulla l'operazione. |
| `/luogo` | Chiede il luogo dell'incontro, con la preposizione davanti (es. "al Vergnano"), massimo 200 caratteri. Il luogo compare nel primo sondaggio. |
| `/cancella_luogo` | Rimuove il luogo dell'incontro. |
| `/debug` | Comando nascosto. Mostra dei pulsanti per eseguire subito: apertura candidature, estrazione, sondaggi dell'incontro, benvenuto. |

## Avvio

Mettere il token del bot nel file `TOKEN` (viene incluso nel binario) oppure passarlo con `-t`.

```
go build && ./cancellierotron [-t TOKEN] [-i URL_PAGINA_INFO]
```

I dati di ogni chat (candidati, luogo, incontro annullato, sondaggio fissato) sono salvati nella cartella di cache dell'utente, sotto `ciambellanotron`.

## Licenza

[WTFPL](LICENSE).
