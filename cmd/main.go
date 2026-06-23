// breakcli - Recordador interactivo de descansos visuales y de movimiento.
// Uso: ./breakcli (Linux) | breakcli.exe (Windows)
// Presiona Ctrl+C para salir.
package main

import (
	"bufio"
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// ─── Configuración ────────────────────────────────────────────────────────────

const (
	workInterval  = 1 * time.Minute
	breakDuration = 30 * time.Second
	promptTimeout = 30 * time.Second
	clearEvery    = 3
)

// ─── Audio embebido ───────────────────────────────────────────────────────────

//go:embed alert.wav
var alertSound []byte

//go:embed alert2.wav
var alertSound2 []byte

// ─── Estado global ────────────────────────────────────────────────────────────

var (
	stdinReader    = bufio.NewReader(os.Stdin)
	breaksComplete = 0
	sessionStart   = time.Now() // ← NUEVO
)

// ─── Helpers ──────────────────────────────────────────────────────────────────

func now() string {
	return time.Now().Format("15:04")
}

func logf(format string, args ...interface{}) {
	fmt.Printf("[%s] %s\n", now(), fmt.Sprintf(format, args...))
}

func clearConsole() {
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("cmd", "/c", "cls")
		cmd.Stdout = os.Stdout
		cmd.Run()
	case "linux":
		cmd := exec.Command("clear")
		cmd.Stdout = os.Stdout
		cmd.Run()
	}
}

// ─── Estadísticas de sesión ───────────────────────────────────────────────────

func fmtDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	case m > 0:
		return fmt.Sprintf("%dm %02ds", m, s)
	default:
		return fmt.Sprintf("%ds", s)
	}
}

func sessionStats() string {
	elapsed := time.Since(sessionStart).Round(time.Second)
	breakAccum := time.Duration(breaksComplete) * breakDuration

	return fmt.Sprintf(
		"  ⏱  Tiempo en el PC         : %s\n"+
			"  🧘  Descanso acumulado      : %s",
		fmtDuration(elapsed),
		fmtDuration(breakAccum),
	)
}

// ─── Bucle principal ──────────────────────────────────────────────────────────

func runBreakLoop() {
	ticker := time.NewTicker(workInterval)
	defer ticker.Stop()
	for range ticker.C {
		doBreak()
	}
}

func doBreak() {
	fmt.Println()
	logf("🔔 ¡Es hora del descanso VISUAL y de MOVIMIENTO!")

	playSound("alert.wav")

	if !askBreak() {
		logf("⏭  Descanso omitido. ¡No olvides descansar pronto! 👀")
		fmt.Println()
		return
	}

	logf("⏸  Descanso iniciado (%dm): levántate, camina y mira lejos. 🧘",
		int(breakDuration.Minutes()))
	fmt.Println()

	runBreakCountdown(breakDuration)

	breaksComplete++ // ← incrementar ANTES de stats

	fmt.Println()
	logf("✅ Fin del descanso VISUAL y de MOVIMIENTO. ¡Vuelve al trabajo! 💪")
	fmt.Println()
	fmt.Println(sessionStats()) // ← NUEVO
	fmt.Println()

	playSound("alert2.wav")

	if breaksComplete%clearEvery == 0 {
		time.Sleep(2 * time.Second)
		clearConsole()
		printBanner()
		logf("🧹 Consola limpiada tras %d descansos completados.", breaksComplete)
		fmt.Println()
	} else {
		fmt.Println()
	}
}

// ─── Prompt interactivo ───────────────────────────────────────────────────────

func askBreak() bool {
	fmt.Printf("[%s] ¿Deseas tomar el descanso ahora? [s/n] (auto en %ds): ",
		now(), int(promptTimeout.Seconds()))

	ch := make(chan string, 1)
	go func() {
		text, err := stdinReader.ReadString('\n')
		if err != nil {
			ch <- ""
			return
		}
		ch <- strings.TrimSpace(strings.ToLower(text))
	}()

	select {
	case ans := <-ch:
		fmt.Println()
		switch ans {
		case "n", "no":
			return false
		default:
			return true
		}
	case <-time.After(promptTimeout):
		fmt.Println()
		logf("⏱  Sin respuesta en %ds. Iniciando descanso automáticamente...",
			int(promptTimeout.Seconds()))
		return true
	}
}

// ─── Cuenta regresiva ─────────────────────────────────────────────────────────

func runBreakCountdown(d time.Duration) {
	total := int(d.Minutes())
	for remaining := total; remaining > 0; remaining-- {
		time.Sleep(time.Minute)
		if remaining-1 > 0 {
			logf("⏳ Descanso en progreso... %d min restante(s).", remaining-1)
		}
	}
}

// ─── Audio ────────────────────────────────────────────────────────────────────

func playSound(filename string) {
	var data []byte
	switch filename {
	case "alert.wav":
		data = alertSound
	case "alert2.wav":
		data = alertSound2
	default:
		logf("[sound] Audio desconocido: '%s'", filename)
		return
	}

	tmpPath, err := writeTempWav(data)
	if err != nil {
		logf("[sound] No se pudo crear archivo temporal: %v", err)
		return
	}
	defer os.Remove(tmpPath)

	switch runtime.GOOS {
	case "windows":
		playSoundWindows(tmpPath, filename)
	case "linux":
		playSoundLinux(tmpPath, filename)
	default:
		logf("[sound] SO '%s' no soportado para audio.", runtime.GOOS)
	}
}

func writeTempWav(data []byte) (string, error) {
	tmp, err := os.CreateTemp("", "breakcli-*.wav")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if _, err := tmp.Write(data); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}

	return tmp.Name(), nil
}

func playSoundWindows(tmpPath, filename string) {
	cmd := exec.Command(
		"powershell", "-NoProfile", "-NonInteractive", "-c",
		fmt.Sprintf(`(New-Object Media.SoundPlayer '%s').PlaySync()`, tmpPath),
	)
	if err := cmd.Run(); err != nil {
		logf("[sound] Error reproduciendo '%s': %v", filename, err)
	}
}

func playSoundLinux(tmpPath, filename string) {
	players := []struct {
		name string
		args []string
	}{
		{"aplay", []string{tmpPath}},
		{"paplay", []string{tmpPath}},
	}

	for _, player := range players {
		if bin, err := exec.LookPath(player.name); err == nil {
			if err := exec.Command(bin, player.args...).Run(); err == nil {
				return
			}
		}
	}

	logf("[sound] Ningún reproductor disponible para '%s'.", filename)
}

// ─── Señales del SO ───────────────────────────────────────────────────────────

func waitForShutdown() {
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
}

// ─── main ─────────────────────────────────────────────────────────────────────

func main() {
	printBanner()
	go runBreakLoop()
	waitForShutdown()
	fmt.Println()
	logf("👋 BreakCLIv2 detenido. ¡Recuerda hacer pausas hoy también!")
	fmt.Println()
}

func printBanner() {
	fmt.Println()
	fmt.Println("╔══════════════════════════════════════════════════╗")
	fmt.Println("║          👁️  BreakCLIv2 — break reminder          ║")
	fmt.Println("╚══════════════════════════════════════════════════╝")
	fmt.Println()
	fmt.Printf("  ✔  Intervalo de trabajo   : cada %d minutos\n", int(workInterval.Minutes()))
	fmt.Printf("  ✔  Duración del descanso  : %d minutos\n", int(breakDuration.Minutes()))
	fmt.Printf("  ✔  Auto-aceptar si no hay respuesta en : %ds\n", int(promptTimeout.Seconds()))
	fmt.Printf("  ✔  Limpiar consola cada   : %d descansos completados\n", clearEvery)
	fmt.Printf("  ✔  SO detectado           : %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  ✔  Descansos completados  : %d\n", breaksComplete)
	fmt.Println()
	fmt.Printf("[%s] 🚀 Iniciando BreakCLIv2... Presiona Ctrl+C para salir.\n", now())
	fmt.Println()
}
