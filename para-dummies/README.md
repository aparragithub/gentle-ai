# Gentle-AI para dummies 🌹

Guías en español, explicadas desde cero, para entender este proyecto punto por punto.
Verificadas contra el código, no escritas de memoria. Cada guía viene en **`.md`** (con
diagramas mermaid) y en **`.pdf`** (con los diagramas ya renderizados).

> Los diagramas de los `.md` están en **mermaid**: se renderizan solos en GitHub, Obsidian
> o VS Code. En los `.pdf` ya vienen dibujados. En texto plano vas a ver el código del diagrama.

## Núcleo — cómo funciona el proyecto

1. **[01-GENTLE-AI-PARA-DUMMIES](01-GENTLE-AI-PARA-DUMMIES.md)** — Qué es, para qué sirve, y qué hace
   cada carpeta. Empezá acá.
2. **[02-SDD-PARA-DUMMIES](02-SDD-PARA-DUMMIES.md)** — El método Spec-Driven Development: las 8 fases,
   cómo se decide el próximo paso, y los backends de persistencia.
3. **[03-REVIEW-PARA-DUMMIES](03-REVIEW-PARA-DUMMIES.md)** — El sistema de revisión acotada: máquina de
   estados, riesgo y lentes, findings, corrección acotada, recibos y los 5 gates.

## Piezas del ecosistema

4. **[04-ENGRAM-PARA-DUMMIES](04-ENGRAM-PARA-DUMMIES.md)** — El sistema de memoria persistente: qué es,
   los conceptos (observaciones/sesiones/prompts), las tools MCP y cómo compartir por git.
5. **[05-COMPONENTS-PARA-DUMMIES](05-COMPONENTS-PARA-DUMMIES.md)** — Qué hace cada componente (engram, sdd,
   skills, persona, context7, permissions, gga, theme) y la persona Gentleman/Neutral **traducida tal cual**.

## Por agente — cómo se integra con cada TUI

6. **[06-CLAUDE-CODE-PARA-DUMMIES](06-CLAUDE-CODE-PARA-DUMMIES.md)** — Qué configura para Claude Code,
   qué archivos toca, qué permite, cómo usa SDD y review.
7. **[07-OPENCODE-PARA-DUMMIES](07-OPENCODE-PARA-DUMMIES.md)** — Ídem para OpenCode: el `opencode.json`,
   los perfiles SDD y los plugins de TUI.
8. **[08-PI-PARA-DUMMIES](08-PI-PARA-DUMMIES.md)** — Ídem para Pi: la instalación de paquetes, el Engram,
   CodeGraph, y el handoff a gentle-pi.
9. **[09-GENTLE-PI-PARA-DUMMIES](09-GENTLE-PI-PARA-DUMMIES.md)** — El tercero `gentle-pi` por dentro: su
   runtime de review nativo, el binario Go verificado, y su relación con Gentle-AI.

## Avanzado / operaciones

10. **[10-ENGRAM-CLOUD-PARA-DUMMIES](10-ENGRAM-CLOUD-PARA-DUMMIES.md)** — Engram Cloud: qué es y **guía de
    puesta en marcha en la empresa** (deploy GHCR, secrets, usuarios gestionados, autosync, troubleshooting).
    Pareja de [04-ENGRAM](04-ENGRAM-PARA-DUMMIES.md).
11. **[11-CODEGRAPH-PARA-DUMMIES](11-CODEGRAPH-PARA-DUMMIES.md)** — CodeGraph (**para todos los agentes**, no solo Pi):
    qué es, en qué ayuda, cómo lo integra y cablea Gentle-AI por agente, y cómo se usa.
99. **[99-OPERACIONES-CLI-PARA-DUMMIES](99-OPERACIONES-CLI-PARA-DUMMIES.md)** — La referencia operativa del CLI:
    install, sync, update/upgrade, backup/restore/rollback, uninstall, doctor.

## El mapa mental en una línea

**Gentle-AI** le pone los muebles a tu agente de IA (memoria, skills, método, persona).
El **SDD** es el método de trabajo (acordar el QUÉ antes del CÓMO). El **Review** hace
verificable esa confianza mediante recibos atados por hash a Git. Cada **agente** (Claude
Code, OpenCode, Pi) recibe esa configuración a su manera; **Pi** es especial porque delega
su runtime en **gentle-pi**, un paquete de terceros.

## ¿Cuál agente está mejor mantenido hoy?

- **Claude Code** — el canónico / de referencia (lo primero que se implementa y prueba).
- **OpenCode** — codo a codo, el más rico en features propias (perfiles, plugins) y el que más
  churning tiene ahora.
- **Pi** — sólido pero el más nuevo; su CodeGraph todavía se está asentando. El paquete
  `gentle-pi` en sí está muy activamente mantenido (releases casi diarias).
