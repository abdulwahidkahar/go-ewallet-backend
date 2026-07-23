package service

import "testing"

func FuzzValidateAndApplyTopUp(f *testing.F) {
	f.Add(int64(1000), int64(500))               // kasus normal
	f.Add(int64(9223372036854775800), int64(50)) // deket batas maksimum int64, harus overflow
	f.Add(int64(9223372036854775807), int64(1))

	f.Fuzz(func(t *testing.T, currentBalance int64, amount int64) {
		if currentBalance < 0 || amount <= 0 {
			return
		}

		newBalance, err := validateAndApplyTopUp(currentBalance, amount)
		if err != nil {
			return
		}

		if newBalance < currentBalance {
			t.Errorf("overflow tidak terdeteksi! currentBalance=%d amount=%d newBalance=%d", currentBalance, amount, newBalance)
		}
	})
}

func FuzzValidateAndApplyTransfer(f *testing.F) {
	f.Add(int64(10000), int64(5000), int64(3000))                            // kasus normal
	f.Add(int64(100), int64(0), int64(9223372036854775807))                  // amount ekstrem, saldo pengirim kecil
	f.Add(int64(9223372036854775807), int64(9223372036854775800), int64(50)) // recipient deket batas maksimum

	f.Fuzz(func(t *testing.T, senderBalance, recipientBalance, amount int64) {
		if senderBalance < 0 || recipientBalance < 0 || amount <= 0 {
			return
		}

		totalBefore := senderBalance + recipientBalance

		newSender, newRecipient, err := validateAndApplyTransfer(senderBalance, recipientBalance, amount)
		if err != nil {
			return
		}

		// PROPERTI 1: saldo pengirim tidak boleh negatif
		if newSender < 0 {
			t.Errorf("saldo pengirim jadi negatif! sender=%d recipient=%d amount=%d newSender=%d", senderBalance, recipientBalance, amount, newSender)
		}

		// PROPERTI 2: saldo penerima harus bertambah, bukan malah berkurang (overflow)
		if newRecipient < recipientBalance {
			t.Errorf("overflow di saldo penerima! sender=%d recipient=%d amount=%d newRecipient=%d", senderBalance, recipientBalance, amount, newRecipient)
		}

		// PROPERTI 3 (paling penting): total uang sebelum dan sesudah harus SAMA PERSIS.
		// Uang cuma boleh pindah tempat, tidak boleh muncul atau hilang.
		totalAfter := newSender + newRecipient
		if totalAfter != totalBefore {
			t.Errorf("uang hilang atau muncul dari mana?! totalBefore=%d totalAfter=%d (sender=%d recipient=%d amount=%d)", totalBefore, totalAfter, senderBalance, recipientBalance, amount)
		}
	})
}
