<?php

namespace App\Ldap\Scopes;

use LdapRecord\Models\Model;
use LdapRecord\Models\Scope;
use LdapRecord\Query\Model\Builder;

class StaffGroups implements Scope
{
    /**
     * Apply the scope to the given query.
     */
    public function apply(Builder $query, Model $model): void
    {
        dd($query->whereMemberOf('CN=8707-gs-All Staff,OU=School Groups,OU=Central,DC=services,DC=education,DC=vic,DC=gov,DC=au')->get());
    }
}
