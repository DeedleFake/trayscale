package locale

// CompilePOtoMOForTest exposes compilePOtoMO for white-box tests.
func CompilePOtoMOForTest(poData []byte) ([]byte, error) {
	return compilePOtoMO(poData)
}

// DGettextForTest exposes dGettext for bind integration tests.
func DGettextForTest(domain, msgid string) string {
	return dGettext(domain, msgid)
}
