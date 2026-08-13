#!/bin/bash

BACKUP_ROOT=/backups

final_dir="$BACKUP_ROOT/$BACKUP_ID"
work_dir="$BACKUP_ROOT/.$BACKUP_ID.partial.$$"

chmod 600 "$work_dir"/*
mv "$work_dir" "$final_dir"
