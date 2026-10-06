# R20 / D35 DOCX verification

WordML tab → TAB and br/cr → LF, including default/textWrapping/page/column
layout breaks. Adjacent styled runs concatenate; existing paragraph LF shares the
bounded separator helper. Inserted bytes count against ParsedBytes before append.
Exact transitional/strict/empty legacy namespace replaces substring recognition;
foreign nodes ignored. This is node-based text extraction, not full OOXML validation.

Both independent acceptances100% (5x20), no unresolved detected errors. Included
public oldAPI baseline probe and parent/reviewer race/lint/probe logs. Baseline
f63179a intentionally fails whitespace in allthree namespace fixtures plus foreign
spoof matching; current repeated public fixtures include explicittextWrapping.
Exact/over extracted byte guards tested with Unicode/entities/combined separators;
ZIP/source/raw expansion/item/wire regressions remain intact.

Public ParsedBytes caps both expanded raw XML and extracted text. Thus a tiny
extracted-text-only boundary is tested directly at the XML helper; public fixtures
verify zipped DOCX behavior while retaining expanded XML bounds. No partial success,
layout reconstruction, fullschema validation, hard CPU/intermediateallocation or
livehostile parser isolation proof is claimed.
