package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Suhare00/mealprep-bot/internal/domain"
	"github.com/Suhare00/mealprep-bot/internal/plan"
)

// MealPlanRepository stores one current plan and bounded recent selections per chat.
type MealPlanRepository struct {
	pool *pgxpool.Pool
}

var _ plan.Repository = (*MealPlanRepository)(nil)

func NewMealPlanRepository(pool *pgxpool.Pool) *MealPlanRepository {
	return &MealPlanRepository{pool: pool}
}

func (r *MealPlanRepository) Load(ctx context.Context, chatID int64) (plan.Stored, error) {
	tx, err := r.pool.BeginTx(ctx, readTransactionOptions())
	if err != nil {
		return plan.Stored{}, fmt.Errorf("begin load meal plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	stored, err := scanStoredPlan(tx.QueryRow(ctx, `
		SELECT breakfast_recipe_id, lunch_recipe_id, dinner_recipe_id,
		       revision, created_at, updated_at
		FROM meal_plans WHERE chat_id = $1`, chatID))
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Stored{}, plan.ErrNotFound
	}
	if err != nil {
		return plan.Stored{}, fmt.Errorf("select meal plan: %w", err)
	}

	rows, err := tx.Query(ctx, `
		SELECT recipe_id
		FROM meal_plan_recent_selections
		WHERE chat_id = $1
		ORDER BY sequence DESC
		LIMIT $2`, chatID, plan.RecentLimit)
	if err != nil {
		return plan.Stored{}, fmt.Errorf("select recent meal-plan recipes: %w", err)
	}
	stored.Recent, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (domain.RecipeID, error) {
		var id string
		if err := row.Scan(&id); err != nil {
			return "", err
		}
		return domain.RecipeID(id), nil
	})
	if err != nil {
		return plan.Stored{}, fmt.Errorf("scan recent meal-plan recipes: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return plan.Stored{}, fmt.Errorf("commit load meal plan transaction: %w", err)
	}
	return stored, nil
}

func (r *MealPlanRepository) Save(ctx context.Context, chatID, expectedRevision int64, ids plan.IDs, at time.Time) (plan.Stored, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return plan.Stored{}, fmt.Errorf("begin save meal plan transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var stored plan.Stored
	if expectedRevision == 0 {
		stored, err = scanStoredPlan(tx.QueryRow(ctx, `
			INSERT INTO meal_plans (
				chat_id, breakfast_recipe_id, lunch_recipe_id, dinner_recipe_id,
				revision, created_at, updated_at
			) VALUES ($1, $2, $3, $4, 1, $5, $5)
			ON CONFLICT (chat_id) DO NOTHING
			RETURNING breakfast_recipe_id, lunch_recipe_id, dinner_recipe_id,
			          revision, created_at, updated_at`,
			chatID, string(ids.Breakfast), string(ids.Lunch), string(ids.Dinner), at))
	} else {
		stored, err = scanStoredPlan(tx.QueryRow(ctx, `
			UPDATE meal_plans SET
				breakfast_recipe_id = $3,
				lunch_recipe_id = $4,
				dinner_recipe_id = $5,
				revision = revision + 1,
				updated_at = $6
			WHERE chat_id = $1 AND revision = $2
			RETURNING breakfast_recipe_id, lunch_recipe_id, dinner_recipe_id,
			          revision, created_at, updated_at`,
			chatID, expectedRevision, string(ids.Breakfast), string(ids.Lunch), string(ids.Dinner), at))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return plan.Stored{}, plan.ErrStaleRevision
	}
	if err != nil {
		return plan.Stored{}, fmt.Errorf("write meal plan: %w", err)
	}

	for _, id := range ids.Slice() {
		if _, err := tx.Exec(ctx, `
			INSERT INTO meal_plan_recent_selections (chat_id, recipe_id, selected_at)
			VALUES ($1, $2, $3)`, chatID, string(id), at); err != nil {
			return plan.Stored{}, fmt.Errorf("insert recent meal-plan recipe: %w", err)
		}
	}
	if _, err := tx.Exec(ctx, `
		DELETE FROM meal_plan_recent_selections
		WHERE chat_id = $1 AND sequence NOT IN (
			SELECT sequence FROM meal_plan_recent_selections
			WHERE chat_id = $1 ORDER BY sequence DESC LIMIT $2
		)`, chatID, plan.RecentLimit); err != nil {
		return plan.Stored{}, fmt.Errorf("trim recent meal-plan recipes: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return plan.Stored{}, fmt.Errorf("commit save meal plan transaction: %w", err)
	}
	stored.Recent = ids.Slice()
	return stored, nil
}

func scanStoredPlan(row rowScanner) (plan.Stored, error) {
	var breakfast, lunch, dinner string
	var stored plan.Stored
	err := row.Scan(&breakfast, &lunch, &dinner, &stored.Revision, &stored.CreatedAt, &stored.UpdatedAt)
	stored.IDs = plan.IDs{
		Breakfast: domain.RecipeID(breakfast),
		Lunch:     domain.RecipeID(lunch),
		Dinner:    domain.RecipeID(dinner),
	}
	return stored, err
}
