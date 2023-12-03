<?php

namespace App\Models;

use Illuminate\Database\Eloquent\Factories\HasFactory;
use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\SoftDeletes;

class Students extends Model
{
    use HasFactory, SoftDeletes;

    public function getLdapDomainColumn()
    {
        return 'domain';
    }

    public function getLdapGuidColumn()
    {
        return 'guid';
    }

    public function setLdapGuid($value)
    {
        $this->attributes['guid'] = $value;
    }

    public function setLdapDomain($value)
    {
        $this->attributes['domain'] = $value;
    }
}
