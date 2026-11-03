package database

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type NodeOutage struct {
	UUID         string
	Name         string
	OfflineSince time.Time
	DownNotified bool
}

type NodeHiddenHost struct {
	UUID   string
	Remark string
}

type NodeOutageRepository struct{ pool *pgxpool.Pool }

func NewNodeOutageRepository(pool *pgxpool.Pool) *NodeOutageRepository {
	return &NodeOutageRepository{pool: pool}
}

func (r *NodeOutageRepository) Start(ctx context.Context, uuid, name string) (NodeOutage, error) {
	var outage NodeOutage
	err := r.pool.QueryRow(ctx, `INSERT INTO node_outage (node_uuid, node_name) VALUES ($1,$2)
		ON CONFLICT (node_uuid) DO UPDATE SET node_name=EXCLUDED.node_name
		RETURNING node_uuid,node_name,offline_since,down_notified`, uuid, name).Scan(&outage.UUID, &outage.Name, &outage.OfflineSince, &outage.DownNotified)
	return outage, err
}

func (r *NodeOutageRepository) Get(ctx context.Context, uuid string) (NodeOutage, bool, error) {
	var outage NodeOutage
	err := r.pool.QueryRow(ctx, `SELECT node_uuid,node_name,offline_since,down_notified FROM node_outage WHERE node_uuid=$1`, uuid).Scan(&outage.UUID, &outage.Name, &outage.OfflineSince, &outage.DownNotified)
	if err == pgx.ErrNoRows {
		return NodeOutage{}, false, nil
	}
	return outage, err == nil, err
}

func (r *NodeOutageRepository) MarkNotified(ctx context.Context, uuid string) error {
	_, err := r.pool.Exec(ctx, `UPDATE node_outage SET down_notified=TRUE WHERE node_uuid=$1`, uuid)
	return err
}

func (r *NodeOutageRepository) RememberHost(ctx context.Context, nodeUUID, hostUUID, remark string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO node_hidden_host(node_uuid,host_uuid,host_remark) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, nodeUUID, hostUUID, remark)
	return err
}

func (r *NodeOutageRepository) HiddenHosts(ctx context.Context, nodeUUID string) ([]NodeHiddenHost, error) {
	rows, err := r.pool.Query(ctx, `SELECT host_uuid,host_remark FROM node_hidden_host WHERE node_uuid=$1 ORDER BY host_uuid`, nodeUUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hosts []NodeHiddenHost
	for rows.Next() {
		var host NodeHiddenHost
		if err := rows.Scan(&host.UUID, &host.Remark); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}
	return hosts, rows.Err()
}

func (r *NodeOutageRepository) ForgetHosts(ctx context.Context, nodeUUID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM node_hidden_host WHERE node_uuid=$1`, nodeUUID)
	return err
}

func (r *NodeOutageRepository) Finish(ctx context.Context, nodeUUID string) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM node_outage WHERE node_uuid=$1`, nodeUUID)
	return err
}
