package database

import "context"

// testUserID é o userID padrão usado pelos testes deste pacote para escopar
// as operações de banco. Existe para que os testes possam usar as APIs
// *WithContext diretamente sem precisar repetir a fixture em cada caso.
const testUserID = "db-test-user"

// testCtx retorna um context.Context já injetado com testUserID, pronto para
// ser passado para qualquer função *WithContext do pacote.
func testCtx() context.Context {
	return WithUserID(context.Background(), testUserID)
}
