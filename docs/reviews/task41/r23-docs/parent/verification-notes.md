# Parent verification scope and initial failures

Root sandboxed race runs failed in TestReleasePreservesUntrackedSource with exit141; the verbose rerun explicitly reports child setpgid: Operation not permitted in the committed bootstrap script. An unsandboxed rerun is required; initial logs remain failures, never PASS evidence.

Initial Docker count3 exposed premature fixture caller deadlines before resource acquisition. Test-only phase-triggered contexts and log-acquired cancellation now verify resources actually owned. The real Policy.LogTimeout clock fixture remains unchanged. First helper attempt called a nil optional callback and failed; final correction retains the nil branch, full Docker race count5 and lint pass. Targeted five phase/clock fixtures race count30 pass. No Docker production policy changed.

Two parent commands accidentally ran root count5 because of incorrect working directory. The second owned process tree was terminated; neither is Docker proof. The first finished with sandboxed release failure. An invalid lint -C invocation and first embedded-field spacing lint failure were corrected by running lint in the actual Docker directory. Final Docker lint reports zero issues.

Initial documentation named-test regex matched English Tests; corrected to require uppercase test name suffix. Verification checks actual func declarations, 240 local target paths and identity of231 tracked non-test production Go files excluding exectool/doc.go. Manifests unchanged.

Correction: unsandboxed root/release also reproduced exit141. Shelltrace isolates git archive | tar, independently hypothesized by acceptance B. Therefore sandbox setpgid is not sufficient explanation. Production bootstrap now writes the entire archive to an owned temporary file, extracts it, and removes the archive. New AAA padding fixture injects16MiB valid zeros through a Git shim, exercises prepare-only actual artifacts, source snapshot equality and no new remote tag. The unchanged bootstrap is checked separately with test overlay; final root and both acceptance reviews must rerun the expanded diff.
