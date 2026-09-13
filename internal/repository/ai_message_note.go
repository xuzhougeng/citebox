package repository

import "database/sql"

func (r *AIConversationRepository) GetNoteSource(conversationID, messageID int64) (AIMessage, string, error) {
	var m AIMessage
	var question string
	err := r.db.QueryRow(`SELECT m.id, m.conversation_id, m.role, m.content,
        COALESCE(m.provider,''), COALESCE(m.model,''), COALESCE(m.mode,''), COALESCE(m.citations_json,''), m.created_at,
        COALESCE((SELECT content FROM ai_messages WHERE conversation_id=m.conversation_id AND role='user' AND id<m.id ORDER BY id DESC LIMIT 1),'')
        FROM ai_messages m WHERE m.conversation_id=? AND m.id=?`, conversationID, messageID).
		Scan(&m.ID, &m.ConversationID, &m.Role, &m.Content, &m.Provider, &m.Model, &m.Mode, &m.CitationsJSON, &m.CreatedAt, &question)
	return m, question, err
}

// One conditional UPDATE preserves concurrent appends and persists deduplication
// with the note itself, without an independent receipt that could outlive deletion.
func (r *PaperRepository) AppendAINote(paperID int64, marker, block string) (bool, error) {
	result, err := r.db.Exec(`UPDATE papers SET paper_notes_text =
        CASE WHEN TRIM(COALESCE(paper_notes_text,''))='' THEN ? ELSE paper_notes_text || char(10) || char(10) || ? END,
        updated_at=CURRENT_TIMESTAMP WHERE id=? AND instr(COALESCE(paper_notes_text,''),?)=0`, block, block, paperID, marker)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil || n > 0 {
		return n > 0, err
	}
	var id int64
	err = r.db.QueryRow(`SELECT id FROM papers WHERE id=?`, paperID).Scan(&id)
	if err == sql.ErrNoRows {
		return false, sql.ErrNoRows
	}
	return false, err
}
